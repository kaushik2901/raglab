# High-Severity Issues — Fix Before Going Live

---

## 1. Chunker Indentation Bug — Uninitialized Variable Use

**File:** `internal/chunker/fixed.go:61-68`

**Why:** The `chunk :=` assignment that tracks the current word-window chunk is at the wrong nesting level. It sits inside the outer `for` loop but outside the inner scope it should occupy. As a result, `chunk` is created with the uninitialized `window` variable, leading to incorrect chunk assignments. This causes:
- Wrong chunk boundaries — text may be split at incorrect offsets.
- Loss of document content in exported chunks.
- Downstream embedding and retrieval of corrupted/wrong text, degrading RAG quality silently.

This is a logic bug, not just a style issue. The code compiles and runs but produces wrong results.

**Fix:** Move the `chunk :=` assignment to the correct scope so it captures the correct `window` slice. The pattern should be:

```go
for _, word := range words {
    currentWindow = append(currentWindow, word)
    for len(currentWindow) >= windowSize {
        chunk := make([]string, windowSize)
        copy(chunk, currentWindow[:windowSize])
        // ... process chunk ...
        currentWindow = currentWindow[1:]
    }
}
```

Review the full chunking logic for correctness and add unit tests that verify chunk boundaries with known inputs.

---

## 2. EOF Comparison by String Instead of `errors.Is`

**Files:** `internal/chunker/fixed.go:41`, `internal/chunker/recursive.go:87`

**Why:** Errors from `ReadElement()` are compared against `io.EOF` using `err.Error() == "EOF"`. This is fragile:
- If any code wraps the error with additional context (e.g. `fmt.Errorf("read element: %w", err)`), the `.Error()` output becomes `"read element: EOF"` and the comparison fails.
- The Go standard library convention is to use `errors.Is(err, io.EOF)`, which unwraps the error chain.
- When this check silently fails, the chunker enters an unexpected control flow — treating EOF as a real error instead of a normal end-of-input signal. This causes infinite loops or spurious error returns.

**Fix:** Replace all `err.Error() == "EOF"` comparisons with `errors.Is(err, io.EOF)`:

```go
if errors.Is(err, io.EOF) {
    break
}
```

---

## 3. Shutdown Uses `context.Background()` — May Hang Forever

**File:** `cmd/workerd/main.go:76`

**Why:** `riverClient.Stop(context.Background())` is called during shutdown with a background context that has no deadline. If River's client is stuck (e.g., waiting for a job to complete, waiting for a database query, or blocked on a channel), `Stop()` hangs indefinitely. The application never terminates, signal handlers don't fire again (unless SIGKILL), and the process becomes a zombie. In containerized environments, this causes slow pod termination, failed rolling updates, and orchestration timeouts.

**Fix:** Use a context with a timeout for graceful shutdown:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := riverClient.Stop(ctx); err != nil {
    slog.Error("river client shutdown", "error", err)
}
```

Same pattern should apply to `s.http.Shutdown(ctx)` in `internal/api/server.go:116` (which currently discards the error).

---

## 4. Default Database Credentials Hardcoded

**Files:** `cmd/workerd/main.go:30`, `internal/db/db.go:16,49`, `internal/config/config.go:66`

**Why:** The default connection string `postgres://rag:rag@localhost:5432/raglab?sslmode=disable` is hardcoded in multiple locations. In production:
- If `DATABASE_URL` is not set (misconfiguration), the service silently connects with these hardcoded credentials.
- The default password `rag` is trivially guessable. Any attacker who can reach the Postgres port can access the database.
- The default uses `sslmode=disable`, transmitting credentials in plaintext.
- Multiple copies of the default string means fixes must be applied in 4+ places.

**Fix:** Remove all default database URLs. In `config.Load()`, require `DATABASE_URL` to be set and return an error if empty. Consolidate the default in exactly one place (config). Use `sslmode=require` or `sslmode=verify-full` for production:

```go
if cfg.DatabaseURL == "" {
    return fmt.Errorf("DATABASE_URL must be set")
}
```

---

## 5. `qdrant/qdrant:latest` Tag in Docker Compose

**File:** `docker-compose.yml:19`

**Why:** The `latest` tag is mutable. Every `docker compose pull` may fetch a different Qdrant version. This causes:
- Non-reproducible deployments — `main` works today but breaks tomorrow after Qdrant release a breaking change.
- Silent data format changes between Qdrant versions.
- Impossible to roll back to a known-good version without manually remembering what version was running last.
- CI/CD pipelines may pass locally but fail after a fresh pull.

**Fix:** Pin to a specific Qdrant version tag, e.g. `qdrant/qdrant:v1.12.0`. Use a `QDRANT_VERSION` environment variable with a documented current version. Update deliberately after testing.

---

## 6. Silently Ignored Unmarshal Errors

**File:** `internal/api/service_eval.go:48,70,101`

**Why:** `json.Unmarshal(metricsJSON, &r.Metrics)` is called three times and the error return value is discarded (assigned to `_`). If the JSON in the database is malformed (due to a bug, partial write, migration, or different version), the metrics silently remain zero-valued. Downstream consumers (dashboards, report generation, comparisons) see inaccurate data with no indication that parsing failed. Debugging requires manual database inspection.

**Fix:** Handle the error explicitly — log it and decide on a per-case strategy (skip, return partial, propagate):

```go
if err := json.Unmarshal(metricsJSON, &r.Metrics); err != nil {
    slog.Warn("failed to unmarshal eval metrics", "run_id", r.ID, "error", err)
    // Continue with zero metrics rather than failing the whole request
}
```

---

## 7. `os.Setenv` in Tests Without Cleanup

**Files:** `internal/generator/factory_test.go`, `internal/embedder/factory_test.go`

**Why:** Tests call `os.Setenv` to set environment variables for factory tests but do not restore the original values. This pollutes the environment for subsequent tests running in the same process (same `go test` binary). Other tests may fail because they inherit stale env vars. When tests are run in random order (`-shuffle=on`), failures become non-deterministic and hard to debug.

**Fix:** Use `t.Setenv(key, value)` instead of `os.Setenv`. The testing framework automatically restores the original value after the test completes (available since Go 1.17):

```go
t.Setenv("LLM_API_KEY", "test-key")
```

Or use `os.Clearenv` + restore in a `t.Cleanup` callback for older patterns.

---

## 8. Real Postgres Credentials in Unit Test

**File:** `internal/eval/store_test.go:18`

**Why:** The eval store test connects to a real Postgres database with hardcoded credentials (`postgres://rag:rag@localhost:5432/raglab?sslmode=disable`). This is a unit test that will fail in CI environments where Postgres is not running, or in developer environments with different database names/credentials. It also violates the project's own convention ("Unit tests must not interact with real infrastructure"). This test is effectively skipped or broken everywhere except the developer's local machine.

**Fix:** Mock the `*pgxpool.Pool` behind an interface, or use `goqu`/`pgxmock` to test SQL query construction without a real database. If an integration test is desired, guard it with `t.Skip("requires Postgres")` and signal it via a build tag (e.g. `//go:build integration`).

---

## 9. No Graceful Job Draining on Workerd Shutdown

**File:** `cmd/workerd/main.go`

**Why:** When the workerd process receives SIGTERM, it immediately starts shutting down River client and closing the database pool. In-flight River jobs (preprocess, index, eval) are interrupted mid-execution. This can leave:
- Partial preprocessing output on disk (incomplete files).
- Partial Qdrant upserts (incomplete vector indices).
- Jobs stuck in the River queue as "running" but will never complete.

River jobs are supposed to be durable, but interrupted jobs may not re-queue properly without additional configuration.

**Fix:** Before calling `riverClient.Stop()`, implement a "drain" phase:
1. Stop accepting new jobs (River's `Stop()` does this).
2. Wait for in-flight jobs to complete with a reasonable timeout.
3. Use River's `Stop(ctx, river.StopOpts{})` which can be configured to wait for active jobs.
4. Ensure the workerd has a `preStop` lifecycle hook in the deployment that gives enough time for graceful shutdown (e.g. `preStop` with `sleep 30` and SIGTERM).

```go
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := riverClient.Stop(shutdownCtx); err != nil {
    slog.Error("failed to stop river client", "error", err)
}
```

---

## 10. `RingBuffer.Purge()` Never Called — Memory Leak

**File:** `internal/memory/memory.go:80-91`

**Why:** `RingBuffer` implements a `Purge()` method but it is never called anywhere in the codebase. The ring buffer is used for per-conversation chat history. Over many conversations, old entries accumulate until the ring buffer reaches capacity, at which point new writes silently overwrite old ones. However, the *conversation map itself* (`conversations map[string]*RingBuffer`) grows without bound. Each unique `conversation_id` creates a new ring buffer entry that is never evicted. Over time, this is a slow memory leak — the map grows proportionally to the number of unique conversation IDs seen since process start.

**Fix:** Implement a TTL-based eviction strategy. Periodically scan the `conversations` map and purge entries that haven't been accessed in N minutes. Use a `sync.Map` or a mutex-protected map with a last-access timestamp. Add a background goroutine (or integration with the cleanup ticker) to evict stale conversations:

```go
type ConversationManager struct {
    mu            sync.RWMutex
    conversations map[string]*conversationEntry
}

type conversationEntry struct {
    buffer    *RingBuffer
    lastAccess time.Time
}
```

---

## 11. `status.FromError(err)` Boolean `ok` Ignored

**File:** `internal/store/qdrant.go:240,271`

**Why:** When checking if a gRPC error is a "collection not found" error, the code uses `status.FromError(err)` but ignores the `ok` return value. If `err` is not a gRPC status error (e.g. a network error, TLS error, or timeout from `google.golang.org/grpc`), `status.FromError` returns `ok=false` and a default `*status.Status`. The code then checks the status code on this default zero-value status, which may falsely match `codes.NotFound` or other codes. This can cause:
- Incorrect handling of network errors as "collection not found" (creates unnecessary new collections).
- Masking real connectivity failures — the error is silently swallowed instead of logged and propagated.
- Hard-to-debug failure modes where Qdrant is down but the application continues with incorrect assumptions.

**Fix:** Always check the `ok` return value:

```go
st, ok := status.FromError(err)
if ok && st.Code() == codes.NotFound {
    // Handle not found
} else if !ok {
    slog.Error("non-gRPC error from Qdrant", "error", err)
    return fmt.Errorf("qdrant communication error: %w", err)
}
```

---

## 12. `http.Server.Shutdown` Return Value Ignored

**File:** `internal/api/server.go:116`

**Why:** The error return value from `s.http.Shutdown(ctx)` is assigned to `_` and discarded. If shutdown fails (e.g., connections refuse to close within the context deadline, or the server is already closed), this failure is invisible. The application assumes a clean shutdown but connections may leak. In orchestrated environments, this can cause port conflicts on restart or stale connections accumulating.

**Fix:** Log the error:

```go
if err := s.http.Shutdown(ctx); err != nil {
    slog.Error("http server shutdown error", "error", err)
}
```
