# Proactive Rate Limiting — Implementation Plan

3 phases. Addresses issue #5 from `known-issues-and-fixes.md`.

---

## Background

Both the embedder and generator use **reactive** rate limiting: when a 429 is received, they sleep with exponential backoff + jitter. This causes:

1. **Thundering herd** — all goroutines hit 429 simultaneously → all sleep → all retry simultaneously → repeat
2. **Uncoordinated** — each goroutine has its own retry state, no shared pacing
3. **Blocked goroutines** — `time.Sleep` blocks, preventing useful work

The fix adds a **token bucket rate limiter** (`golang.org/x/time/rate`) that proactively limits request rate so 429s are rare. The rate limiter is a shared wrapper, so all goroutines using the same embedder/generator instance share the same bucket.

### Batch-size invariant (important)

The embedder wraps `Embed` (not `embedBatch`). This is correct because:

| Caller | `len(chunks)` passed to `Embed` | Embedder's internal `batchSize` | API calls per `Embed` call |
|--------|-------------------------------|-------------------------------|---------------------------|
| Index worker (`processFile`) | `batchSize` (config, default 20) | Same `batchSize` | 1 |
| Eval pipeline (`batchEmbedQueries`) | `batchSize` (config) | Same `batchSize` | 1 |

Since the top-level batch size and the embedder's internal sub-batch size are **always the same value** (both come from `args.BatchSize`), `Embed` makes exactly 1 API call per invocation. If this invariant changes in the future, the rate limiter must be moved into `embedBatch`.

### RPM=0 means "no rate limiting"

If `EMBEDDER_RATE_LIMIT_RPM` or `GENERATOR_RATE_LIMIT_RPM` is set to `0` (or negative), no rate limiter is applied — the inner embedder/generator is returned directly.

---

## Phase 0: `RateLimitedEmbedder` wrapper

**Files to create:**
- `internal/embedder/ratelimit.go`

**What it contains:**

```go
package embedder

import (
    "context"
    "golang.org/x/time/rate"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type RateLimitedEmbedder struct {
    inner  Embedder
    bucket *rate.Limiter
}

func NewRateLimitedEmbedder(inner Embedder, rpm float64) *RateLimitedEmbedder {
    limit := rate.Limit(rpm / 60.0)
    burst := int(rpm / 60)
    if burst < 1 {
        burst = 1
    }
    return &RateLimitedEmbedder{
        inner:  inner,
        bucket: rate.NewLimiter(limit, burst),
    }
}

func (r *RateLimitedEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
    if err := r.bucket.Wait(ctx); err != nil {
        return nil, err
    }
    return r.inner.Embed(ctx, chunks)
}

func (r *RateLimitedEmbedder) Dimensions() int  { return r.inner.Dimensions() }
func (r *RateLimitedEmbedder) ModelName() string { return r.inner.ModelName() }
```

**Key design decisions:**
- `Wait` blocks goroutine until a token is available or context is cancelled. Goroutines **queue up** at the limiter instead of all hitting the API.
- Burst is 1 second worth of tokens (minimum 1). Prevents unnecessary serialization for small bursts.
- `inner.Embed` is called after token acquisition. The reactive 429 retry inside `embedBatch` remains as a safety net.

**`golang.org/x/time/rate`:** Add via `go get golang.org/x/time/rate`. Check `go.mod` first.

**Tests (`internal/embedder/ratelimit_test.go`):**

```
TestRateLimitedEmbedder_Delegation
    Mock inner Embedder
    Call Embed → verify inner.Embed called with same chunks
    Call Dimensions/ModelName → verify delegated

TestRateLimitedEmbedder_HighRPM
    Create with 100000 RPM
    Embed 10 batches → all succeed immediately

TestRateLimitedEmbedder_ZeroRPM
    Create with 0 RPM
    Verify NewRateLimitedEmbedder returns the inner embedder unwrapped
```

**Verify:** `go test ./internal/embedder/`

---

## Phase 1: `RateLimitedGenerator` wrapper

**Files to create:**
- `internal/generator/ratelimit.go`

**What it contains:**

```go
package generator

import (
    "context"
    "golang.org/x/time/rate"
    "github.com/openai/openai-go"
)

type RateLimitedGenerator struct {
    inner  Generator
    bucket *rate.Limiter
}

func NewRateLimitedGenerator(inner Generator, rpm float64) *RateLimitedGenerator {
    if rpm <= 0 {
        return inner.(*RateLimitedGenerator) // actually just return inner
        // Wait, we need to return Generator interface
    }
    // ... same pattern as embedder
}
```

Wait — the "unwrap on zero RPM" pattern for the generator must return `Generator`, not `*RateLimitedGenerator`. If RPM <= 0, just return the inner `Generator` directly:

```go
func NewRateLimitedGenerator(inner Generator, rpm float64) Generator {
    if rpm <= 0 {
        return inner
    }
    limit := rate.Limit(rpm / 60.0)
    burst := int(rpm / 60)
    if burst < 1 {
        burst = 1
    }
    return &RateLimitedGenerator{
        inner:  inner,
        bucket: rate.NewLimiter(limit, burst),
    }
}
```

**Note:** `NewRateLimitedGenerator` returns `Generator` (the interface), not the concrete type. This allows returning the unwrapped inner when RPM <= 0.

**Tests (`internal/generator/ratelimit_test.go`):**

```
TestRateLimitedGenerator_Delegation
    Mock inner Generator
    Call Generate → verify inner.Generate called with same params
    Call ModelName → delegated

TestRateLimitedGenerator_HighRPM
    Create with 100000 RPM
    Generate 10 times → all succeed

TestRateLimitedGenerator_ZeroRPM
    Create with 0 RPM
    Verify returned value is the inner generator (not RateLimitedGenerator)
```

**Verify:** `go test ./internal/generator/`

---

## Phase 2: Wire rate limiters into `New()` factories

**Files to modify:**
- `internal/embedder/embedder.go`
- `internal/generator/generator.go`
- `internal/config/config.go`

### `FloatEnvOrDefault` helper (config.go)

```go
func FloatEnvOrDefault(key string, defaultVal float64) float64 {
    if v := os.Getenv(key); v != "" {
        if f, err := strconv.ParseFloat(v, 64); err == nil {
            return f
        }
    }
    return defaultVal
}
```

### `embedder.New` changes

```go
func New(provider config.Provider, model string, batchSize int) (Embedder, error) {
    baseURL, apiKey := config.ResolveProviderConfig(provider)
    if baseURL == "" {
        return nil, fmt.Errorf("empty base URL for provider %q", provider)
    }
    e := newOpenAIEmbedder(baseURL, apiKey, model, batchSize)
    rpm := config.FloatEnvOrDefault("EMBEDDER_RATE_LIMIT_RPM", 100)
    if rpm > 0 {
        e = NewRateLimitedEmbedder(e, rpm) // returns *RateLimitedEmbedder which implements Embedder
    }
    return e, nil
}
```

Wait — `newOpenAIEmbedder` returns `*embedder` which implements `Embedder`. But `NewRateLimitedEmbedder` returns `*RateLimitedEmbedder` which also implements `Embedder`. So the variable must be `Embedder`:

```go
func New(provider config.Provider, model string, batchSize int) (Embedder, error) {
    baseURL, apiKey := config.ResolveProviderConfig(provider)
    if baseURL == "" {
        return nil, fmt.Errorf("empty base URL for provider %q", provider)
    }
    var e Embedder = newOpenAIEmbedder(baseURL, apiKey, model, batchSize)
    rpm := config.FloatEnvOrDefault("EMBEDDER_RATE_LIMIT_RPM", 100)
    if rpm > 0 {
        e = NewRateLimitedEmbedder(e, rpm)
    }
    return e, nil
}
```

### `generator.New` changes

```go
func New(provider config.Provider, model string) (Generator, error) {
    baseURL, apiKey := config.ResolveProviderConfig(provider)
    if baseURL == "" {
        return nil, fmt.Errorf("empty base URL for provider %q", provider)
    }
    gen := NewOpenAI(baseURL, apiKey, model)
    rpm := config.FloatEnvOrDefault("GENERATOR_RATE_LIMIT_RPM", 100)
    return NewRateLimitedGenerator(gen, rpm), nil
}
```

`NewRateLimitedGenerator` already handles the rpm <= 0 case by returning `gen` unwrapped, so no conditional needed at the call site.

### Tests

```
TestEmbedderFactory_WrapsWithRateLimiter
    Call embedder.New with valid params and EMBEDDER_RATE_LIMIT_RPM=100
    Verify returned Embedder is a *RateLimitedEmbedder

TestEmbedderFactory_NoRateLimiter
    Set EMBEDDER_RATE_LIMIT_RPM=0
    Call embedder.New
    Verify returned Embedder is NOT a *RateLimitedEmbedder

TestGeneratorFactory_WrapsWithRateLimiter
    Call generator.New with valid params and GENERATOR_RATE_LIMIT_RPM=100
    Verify returned Generator is a *RateLimitedGenerator

TestGeneratorFactory_NoRateLimiter
    Set GENERATOR_RATE_LIMIT_RPM=0
    Call generator.New
    Verify returned Generator is NOT a *RateLimitedGenerator
```

**Verify:** `go test ./internal/embedder/ ./internal/generator/`

---

## Dependency graph

```
Phase 0 (RateLimitedEmbedder)  ──►  Phase 2 (wire into New() factories)
Phase 1 (RateLimitedGenerator) ──┘

No dependency between Phase 0 and Phase 1 — can be done in parallel.
```

---

## Testing strategy summary

| Phase | What's tested | How | Key edge cases |
|-------|--------------|-----|----------------|
| 0 | Wrapper delegates to inner + rate limits | High RPM → all pass; mock inner → verify delegation; 0 RPM → unwrapped | Context cancellation during Wait, RPM=0 |
| 1 | Same pattern as Phase 0 for generator | Same approach | RPM=0 returns inner directly (interface return type) |
| 2 | Factory methods wrap; `FloatEnvOrDefault` | Direct call to `New()`, env var override | Missing env var → default 100 RPM, invalid env var value → default |

---

## Open questions (resolved)

| Question | Resolution |
|----------|-----------|
| Should the rate limiter wrap `Embed` or `embedBatch`? | **`Embed`** — analysis shows each `Embed` call makes exactly 1 API call in all current call sites because top-level `batchSize` matches the embedder's internal `batchSize`. |
| What does RPM=0 mean? | **No rate limiting** — the inner impl is returned directly (no wrapper). |
| Default RPM? | **100** — matches OpenAI free-tier RPM for `text-embedding-3-small`. Users with higher tiers should set `EMBEDDER_RATE_LIMIT_RPM` and `GENERATOR_RATE_LIMIT_RPM` accordingly. |
| Does the rate limiter replace the 429 retry? | **No** — the reactive retry remains as a safety net. The proactive limiter makes 429s rare; the reactive retry handles residual ones. |
| Is `golang.org/x/time/rate` already a dependency? | Check `go.mod`; if not present, add via `go get`. |

---

## Risk assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Rate limiter applied at wrong granularity (Embed vs embedBatch) | Low | Incorrect behavior if batch sizes diverge | Documented invariant; code review guard |
| RPM=0 or negative causes panic | Low | Crash | `rate.NewLimiter(0, burst)` creates an infinite-rate limiter, not a crash. But Wait blocks forever after burst consumed. The plan handles RPM <= 0 by returning inner directly. |
| `FloatEnvOrDefault` parse failure | Low | Falls back to default | Second return value from `strconv.ParseFloat` is checked |
| Context cancellation during Wait leaks goroutines | Medium | Goroutine blocked | `Wait` unblocks when ctx is cancelled. Safe. |
