# Unit Testing Gaps & Testability Analysis

> Generated: 2026-06-10 (updated for API-first architecture changes)
> Analysis covers all non-test Go source files across the project.

---

## Table of Contents

1. [Files Without Tests](#1-files-without-tests)
2. [Testability Anti-Patterns & Violations](#2-testability-anti-patterns--violations)
3. [Existing Test Quality Issues](#3-existing-test-quality-issues)
4. [Hardcoded Infrastructure Dependencies](#4-hardcoded-infrastructure-dependencies)
5. [Missing Interface Abstractions](#5-missing-interface-abstractions)
6. [Recommended Actions by Priority](#6-recommended-actions-by-priority)

---

## 1. Files Without Tests

### 1.1 `cmd/` — CLI entry points (2 files remaining)

The API-first plan deleted 4 CLI binaries (`cmd/preprocess/`, `cmd/index/`, `cmd/query/`, `cmd/eval/`). Only `api` and `workerd` remain.

| File | Reason for gap |
|------|---------------|
| `cmd/api/main.go` | Thin wrapper; creates config, connects to DB/Qdrant, starts HTTP server. Infra-heavy constructor makes unit testing impossible. |
| `cmd/workerd/main.go` | Creates River client, registers workers, starts serving. |

**Why untestable:** Both call `config.Load()` (uses global `flag`), connect to infrastructure (Postgres, Qdrant), and wire concrete dependencies in `main()`. These are thin wiring entry points — acceptable to keep as untested if all business logic is tested downstream.

### 1.2 `internal/api/` — Routers, services, and server (8 files, uneven coverage)

| File | Gap |
|------|-----|
| `internal/api/router_artifact.go` | Handler (`listHandler`) is tested in `handler_artifact_test.go`, but router struct/`Register` is not. |
| `internal/api/router_chat.go` | `chatStreamHandler` is tested in `handler_chat_stream_test.go`, but `chatHandler` and `Register` are not. |
| `internal/api/router_eval.go` | **No tests at all.** |
| `internal/api/router_health.go` | `healthHandler` is tested in `handler_health_test.go`, but `Register` is not. |
| `internal/api/router_workflow.go` | All handlers tested in `handler_workflow_test.go`, but `Register` is not. |
| `internal/api/server.go` | **No tests.** `New()` connects to Postgres and Qdrant during construction — impossible to unit test. |
| `internal/api/service_chat.go` | **No tests** for `Chat()`, `ChatStream()`, `buildMessages()`, `retrieveSources()`. The refactored code with injectable factory functions (`newEmbedderFn`, `newRetrieverFn`, `newGeneratorFn`) is now **very testable** — this is a high-priority gap. |
| `internal/api/service_eval.go` | **No tests.** Takes `*pgxpool.Pool` directly; all methods query Postgres. |
| `internal/api/types.go` | Validation methods are tested via handler tests, but not in **isolated unit tests**. The API-first plan added extensive validation logic that needs direct testing. |

### 1.3 `internal/config/` (1 file untested)

| File | Gap |
|------|-----|
| `internal/config/normalize.go` | `NormalizeBaseURL`, `WarnOnInsecure`, `ParseRetryAfter` — all are **pure functions** with no tests. These are textbook unit-testable functions. |

### 1.4 `internal/db/` (1 file untested)

| File | Gap |
|------|-----|
| `internal/db/db.go` | `Connect()` and `NewRiverClient()` connect to a real Postgres instance. No interface abstraction exists. |

### 1.5 `internal/eval/` (2 files untested)

| File | Gap |
|------|-----|
| `internal/eval/pipeline.go` | Contains only the `VectorSearcher` interface and `SystemPrompt` constant — minimal code. |
| `internal/eval/worker.go` | `EvaluateQuestion()`, `toFloat32()`, `fillRetrievalResult()`, `buildContextText()`, `generateForQuestion()` — **all pure logic functions** that can and should be tested without any infrastructure. These are the core eval pipeline functions. |

### 1.6 `internal/workflow/` (1 file untested)

| File | Gap |
|------|-----|
| `internal/workflow/poll_job.go` | `PollUntilTerminal()` — a standalone polling function that depends on `river.Client`. Could be tested with a mock client, but currently has no tests. |

---

## 2. Testability Anti-Patterns & Violations

### CRITICAL: Infrastructure Dependencies Wired in Constructors

Several constructors create real infrastructure connections, making them impossible to unit-test:

```go
// internal/api/server.go:30 — Connects to Postgres AND Qdrant in constructor
func New(cfg *config.Config) (*Server, error) {
    pool, err := db.Connect(context.Background(), cfg.DatabaseURL)  // REAL POSTGRES
    qdrantStore := qstore.NewQdrantStore(cfg.QdrantAPIKey)
    qdrantStore.Connect(context.Background(), cfg.QdrantURL)        // REAL QDRANT
    ...
}
```

```go
// internal/api/service_eval.go:16 — Takes *pgxpool.Pool, no interface
func NewEvalService(pool *pgxpool.Pool) *EvalService { ... }
```

**Note:** `service_chat.go` was fixed by the API-first plan — it now uses injectable factory functions. `server.go` and `service_eval.go` remain blockers.

### CRITICAL: Global State (`flag` + `os.Getenv`)

```go
// internal/config/config.go:37 — Uses global flag.FlagSet
func Load() (*Config, error) {
    flag.IntVar(&cfg.MaxRetries, "max-retries", IntEnvOrDefault(...), ...)
    flag.Parse()  // ← contaminates global flag state
    ...
}
```

Every function in `config.go` reads directly from `os.Getenv()`:
- `EnvOrDefault()`, `IntEnvOrDefault()`, `DurationEnvOrDefault()`, `FloatEnvOrDefault()`
- `ResolveProviderConfig()`

Tests cannot safely run in parallel — they must coordinate env var state with `t.Setenv()` and reset flags.

**Note:** API-first plan slated `ResolveTag` and `FloatEnvOrDefault` for removal but they are still present in `config.go` (unused dead code).

### HIGH: Missing Interface for Postgres Pool

`*pgxpool.Pool` is used directly everywhere:
- `internal/eval/store.go` — `EvalStore.pool *pgxpool.Pool`
- `internal/api/service_eval.go` — `EvalService.pool *pgxpool.Pool`

No interface abstraction means all code using these types requires a real Postgres instance to test.

### HIGH: `time.Sleep()` in Rate-Limit Retry Logic

```go
// internal/embedder/openai.go:97
time.Sleep(backoff + jitter)

// internal/generator/generator.go:84
time.Sleep(backoff + jitter)
```

Real wall-clock sleeps in retry loops make tests slow and flaky. The existing tests work around this by setting `retryBackoff` to `time.Millisecond`, but the sleep is still real.

### HIGH: Command Execution in Production Code

```go
// internal/workflow/preprocess_worker.go:91
cmd := exec.CommandContext(ctx, "git", "clone", ...)
cmd.Run()
```

Git operations are executed directly in worker code, making the `PreprocessWorker.Work()` method impossible to unit test without mocking `exec.Command`.

### MEDIUM: Worker `createEvalDeps()` and `RunIndexing()` create real infra

```go
// internal/workflow/eval_worker.go:336
func createEvalDeps(ctx context.Context, args EvalArgs) (embedder.Embedder, *qstore.QdrantStore, ...) {
    qdrantURL := os.Getenv("QDRANT_URL")      // real env
    qStore := qstore.NewQdrantStore(qdrantAPIKey)
    qStore.Connect(ctx, qdrantURL)             // real Qdrant
    emb, err := embedder.New(...)              // real API call
    ...
}
```

Same pattern in `index_worker.go:RunIndexing()`.

---

## 3. Existing Test Quality Issues

### 3.1 Integration Tests Masked as Unit Tests (need real infra)

| File | Issue |
|------|-------|
| `internal/eval/store_test.go` | All tests require a running Postgres via `connectOrSkip("postgres://...")`. If Postgres is down, tests silently skip. These are **integration tests** living in the wrong place. |
| `internal/workflow/eval_worker_test.go` | Test tests nonexistent dataset path — fragile. |
| `internal/workflow/test_helpers_test.go` | `connectOrSkip` requires real Postgres. |
| `internal/workflow/index_worker_test.go` | Tests nonexistent input dir — fragile and doesn't test the actual logic. |
| `internal/store/qdrant_test.go` | **6 tests** using `t.Skip("requires Qdrant server")` — these are not unit tests. |

### 3.2 No Parallel Test Execution

A search across all test files shows **zero** calls to `t.Parallel()`. The existing tests:
- Can't safely run in parallel due to global flag/env state
- Don't benefit from multi-core speedup
- Would likely fail under `-parallel` due to shared env vars

### 3.3 Handler Tests Not Updated for API-First Validation

The API-first plan added extensive `Validate()` logic to all request types (`PreprocessRequest`, `IndexRequest`, `EvalRequest`, `ChatRequest`). The handler tests in `handler_workflow_test.go` and `handler_chat_stream_test.go` need updating to send valid payloads with all required fields.

### 3.4 Good Test Patterns to Follow

- `internal/eval/retrieval_test.go` — mock interfaces, tests pure logic, no infra required
- `internal/embedder/openai_test.go` — uses `httptest.NewServer`, tests all error paths
- `internal/chunker/fixed_test.go` — thorough table-driven tests

---

## 4. Hardcoded Infrastructure Dependencies

| Location | Dependency | Fix |
|----------|-----------|-----|
| `internal/db/db.go:14` | `pgxpool.New(ctx, dsn)` | Accept `pgxpool.Pool` as parameter; let caller connect |
| `internal/db/db.go:30` | `river.NewClient(...)` | Same |
| `internal/store/qdrant.go:29` | `qdrant.NewGrpcClient(...)` | `QdrantStore` already implements `VectorStore`; callers should accept the interface |
| `internal/api/server.go:31` | `db.Connect()` | Accept `*pgxpool.Pool` and `VectorStore` as params instead of creating them |
| `internal/api/service_eval.go:16` | `*pgxpool.Pool` | Define an `EvalDB` interface with the needed methods |
| `internal/workflow/eval_worker.go:336` | `createEvalDeps()` | Accept embedder, Qdrant, generator as parameters |
| `internal/workflow/index_worker.go:123` | `RunIndexing()` | Accept dependencies as params |
| `internal/workflow/preprocess_worker.go:82` | `cloneRepo()` | Define `GitRunner` interface |
| `internal/embedder/openai.go:38` | `openai.NewClient(opts...)` | Embedder interface is good; tests use `httptest.Server`. Real issue is `embedder.New()` reads env vars. |
| `internal/generator/generator.go:46` | `openai.NewClient(opts...)` | Same as embedder |

---

## 5. Missing Interface Abstractions

### 5.1 Postgres — No Abstraction

```go
// Current: concrete type everywhere
type EvalStore struct { pool *pgxpool.Pool }
type EvalService struct { pool *pgxpool.Pool }

// Missing: interface that would enable mocking
type EvalDB interface {
    CreateRun(ctx, tag, strategy) (string, error)
    BulkAddQueryResults(ctx, runID, results) error
    UpdateRunMetrics(ctx, runID, metrics) error
    DeleteRunResults(ctx, runID) error
    GetRunResults(ctx, runID) ([]Result, error)
}
```

### 5.2 File System — No Abstraction for Walk/Read/Write

`internal/workflow/preprocess_worker.go` uses `os.ReadFile`, `filepath.Walk`, `os.Stat`, `os.MkdirAll`, etc. directly.

### 5.3 Git Command — No Abstraction

`internal/workflow/preprocess_worker.go` uses `exec.CommandContext` for git operations.

---

## 6. Recommended Actions by Priority

### P0 — Must Fix for Unit Testability (blockers)

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | `internal/config/config.go:37` | Global `flag` in `Load()` | Use private `*flag.FlagSet` parameter; never call `flag.Parse()` in library code |
| 2 | `internal/config/config.go` | Direct `os.Getenv()` calls | Inject env via `EnvLookup func(string) string` parameter |
| 3 | `internal/api/server.go:30` | Connects to Postgres/Qdrant in constructor | Accept connections via parameters (dependency injection) |
| 4 | `internal/api/service_eval.go:16` | `*pgxpool.Pool` directly | Define `EvalDB` interface, inject it |
| 5 | `internal/workflow/eval_worker.go:336` | `createEvalDeps()` creates real infra | Accept embedder, Qdrant, generator as parameters |
| 6 | `internal/workflow/index_worker.go:123` | `RunIndexing()` creates real infra | Accept dependencies as parameters |

### P1 — Tests Missing for Testable Code (pure logic, no infra needed)

| # | File | Functions | Priority |
|---|------|-----------|----------|
| 1 | `internal/config/normalize.go` | `NormalizeBaseURL`, `WarnOnInsecure`, `ParseRetryAfter` | **HIGH** — pure functions, no deps |
| 2 | `internal/eval/worker.go` | `EvaluateQuestion`, `toFloat32`, `fillRetrievalResult`, `buildContextText`, `generateForQuestion` | **HIGH** — core eval pipeline logic |
| 3 | `internal/api/types.go` | `Validate()` methods for all 4 request types | **HIGH** — new validation logic from API-first plan, currently untested in isolation |
| 4 | `internal/api/service_chat.go` | `Chat()`, `ChatStream()`, `buildMessages()`, `retrieveSources()` | **HIGH** — refactored with injectable factories, ready for mock-based tests |
| 5 | `internal/config/config.go` | `Config.Validate()`, `ResolveProviderConfig()` | **MEDIUM** — config validation logic |
| 6 | `internal/workflow/poll_job.go` | `PollUntilTerminal` | **MEDIUM** — needs mock client |
| 7 | `internal/eval/pipeline.go` | `VectorSearcher` interface + `SystemPrompt` | **LOW** — trivial code |

### P2 — Improve Existing Test Quality

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | All test files | No `t.Parallel()` | Add after fixing global state issues |
| 2 | `internal/eval/store_test.go` | Integration test (needs Postgres) | Move to `store_integration_test.go` with build tag; split interface |
| 3 | `internal/store/qdrant_test.go` | 6 tests skipped (`t.Skip`) | Move integration tests out; keep unit-testable helpers |
| 4 | `internal/workflow/eval_worker_test.go` | Tests nonexistent dataset | Add proper mock-based tests |
| 5 | `internal/workflow/index_worker_test.go` | Tests non-existent dir | Add proper mock-based tests for `processFile`, `embedAndStore` |
| 6 | `internal/api/handler_workflow_test.go` | Payloads missing required fields | Update to match new API-first validation |
| 7 | `internal/api/handler_chat_stream_test.go` | Payloads missing required fields | Same |

### P3 — Structural Improvements

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | `internal/embedder/openai.go:97` | `time.Sleep()` in retry | Make backoff function injectable |
| 2 | `internal/generator/generator.go:84` | `time.Sleep()` in retry | Same |
| 3 | `internal/workflow/preprocess_worker.go:82-124` | Direct git command execution | Define `GitRunner` interface |
| 4 | `internal/workflow/preprocess_worker.go:142-398` | Direct file system calls | Define `FileSystem` interface |
| 5 | `internal/config/config.go` | Unused `ResolveTag`, `FloatEnvOrDefault` | Remove dead code (already flagged by API-first plan) |

---

## Summary Statistics

| Metric | Value |
|--------|-------|
| Total non-test Go source files | ~55 (4 CLIs deleted by API-first plan) |
| Files with no test file | ~22 (reduced from 27 by API-first deletions) |
| Files with inadequate tests | ~10 (integration-only or too trivial) |
| Test files with integration dependencies | 5 (eval/store, workflow/*, store/qdrant) |
| Pure functions untested | 10+ (`normalize.go`, `eval/worker.go`, `poll_job.go`, `types.go` validation) |
| Tests calling `t.Parallel()` | **0** |
| Uses of `t.Skip("requires ...")` | 6 (all in `qdrant_test.go`) |
| Can run tests without Postgres/Qdrant today | ~70% of test files |

### API-First Plan Impact on Testability

| Change | Testability Impact |
|--------|-------------------|
| ✅ Deleted 4 CLI binaries | Removed 4 untestable files from the gap list |
| ✅ `service_chat.go` refactored with factory functions | **Much more testable** — injectable factories replace hard-coded env reads |
| ✅ Added `Validate()` methods (all fields required) | New logic that needs tests, but makes code more explicit |
| ✅ Worker fallback defaults removed | Fewer code paths to test |
| ❌ `config.go` still uses `flag.Parse()` + `os.Getenv` | No change — still the #1 blocker |
| ❌ `config.go` still has dead `ResolveTag`, `FloatEnvOrDefault` | Just cleanup needed |

---

*Generated by analyzing ~55 source files and ~39 test files across ~18 packages.*
