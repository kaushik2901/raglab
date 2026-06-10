# Unit Testing Gaps & Testability Analysis

> Generated: 2026-06-10
> Analysis covers all 59 non-test Go source files across 20 packages.

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

### 1.1 `cmd/` — CLI entry points (5 files, 0% coverage)

| File | Reason for gap |
|------|---------------|
| `cmd/api/main.go` | Thin wrapper; creates config, connects to DB/Qdrant, starts HTTP server. Infra-heavy constructor makes unit testing impossible. |
| `cmd/eval/main.go` | Reads dataset files, creates River client, inserts eval jobs, polls for results. No abstraction layer. |
| `cmd/query/main.go` | Creates embedder/generator, reads Qdrant, streams responses. All dependencies hard-wired. |
| `cmd/workerd/main.go` | Creates River client, registers workers, starts serving. |
| `cmd/index/main.go` | Has `main_test.go` but only tests `ResolveTag`; the main logic is untested. |
| `cmd/preprocess/main.go` | Same as index — `main_test.go` only tests tag resolution. |

**Why untestable:** All `cmd/main.go` files call `config.Load()` (uses global `flag`), connect to infrastructure (Postgres, Qdrant), and wire concrete dependencies in `main()`. There is no dependency injection — the entire wiring happens in a single monolithic function.

### 1.2 `internal/api/` — Routers, services, and server (8 files, 0-46% coverage)

| File | Gap |
|------|-----|
| `internal/api/router_artifact.go` | Handler (`listHandler`) is tested in `handler_artifact_test.go`, but router struct/`Register` is not. |
| `internal/api/router_chat.go` | `chatStreamHandler` is tested in `handler_chat_stream_test.go`, but `chatHandler` and `Register` are not. |
| `internal/api/router_eval.go` | No tests at all. |
| `internal/api/router_health.go` | `healthHandler` is tested in `handler_health_test.go`, but `Register` is not. |
| `internal/api/router_workflow.go` | All handlers tested in `handler_workflow_test.go`, but `Register` is not. |
| `internal/api/server.go` | **No tests.** `New()` connects to Postgres and Qdrant during construction — impossible to unit test. |
| `internal/api/service_chat.go` | **No tests.** `NewChatService()` creates real embedders/generators/retrievers from env vars. |
| `internal/api/service_eval.go` | **No tests.** Takes `*pgxpool.Pool` directly; all methods query Postgres. |
| `internal/api/types.go` | Validation methods are tested via handler tests, but not in isolation. |

### 1.3 `internal/chunker/` (1 file, 100% — only structural)

| File | Gap |
|------|-----|
| `internal/chunker/chunker.go` | `New()` factory and `RegisterChunker()` are covered by `registry_test.go`. Adequate. |

### 1.4 `internal/config/` (1 file untested)

| File | Gap |
|------|-----|
| `internal/config/normalize.go` | `NormalizeBaseURL`, `WarnOnInsecure`, `ParseRetryAfter` — all are **pure functions** with no tests. These are textbook unit-testable functions. |

### 1.5 `internal/db/` (1 file untested)

| File | Gap |
|------|-----|
| `internal/db/db.go` | `Connect()` and `NewRiverClient()` connect to a real Postgres instance. No interface abstraction exists. |

### 1.6 `internal/embedder/` (1 file, interface only)

| File | Gap |
|------|-----|
| `internal/embedder/embedder.go` | `New()` factory function falls back to env var. Tested via `factory_test.go` which uses `FloatEnvOrDefault` mock implicitly. Adequate. |

### 1.7 `internal/eval/` (2 files untested)

| File | Gap |
|------|-----|
| `internal/eval/pipeline.go` | Contains only the `VectorSearcher` interface and `SystemPrompt` constant — minimal code, but the interface definition is unused in tests. |
| `internal/eval/worker.go` | `EvaluateQuestion()`, `toFloat32()`, `fillRetrievalResult()`, `buildContextText()`, `generateForQuestion()` — **all pure logic functions** that can and should be tested without any infrastructure. These are the core eval pipeline functions. |

### 1.8 `internal/parser/` (1 file gap — but covered)

| File | Gap |
|------|-----|
| `internal/parser/parser.go` | Factory/registry tests exist in `registry_test.go`. Adequate. |

### 1.9 `internal/store/` (1 file gap)

| File | Gap |
|------|-----|
| `internal/store/store.go` | Contains only the `VectorStore` interface definition. Trivial. |

### 1.10 `internal/types/` (2 files untested)

| File | Gap |
|------|-----|
| `internal/types/document.go` | `Document` struct — trivial, but should have a basic creation test for consistency. |
| `internal/types/eval.go` | `RelevanceJudgment`, `EvalQuestion`, `RetrievalResult`, `AggregateMetrics` — struct definitions, tested indirectly via other tests. |
| `internal/types/query.go` | `SearchResult` — tested via `types_test.go`. Adequate. |

### 1.11 `internal/workflow/` (1 file untested)

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
// internal/api/service_chat.go:31 — Creates real embedder, retriever, generator from env
func NewChatService(cfg *config.Config, vs qstore.VectorStore) (*ChatService, error) {
    emb, err := embedder.New(...)        // REAL EMBEDDING API CALL
    ret, err := retriever.New(emb, vs, ...)  // REAL RETRIEVER
    gen, err := generator.New(...)       // REAL LLM CALL
}
```

```go
// internal/api/service_eval.go:16 — Takes *pgxpool.Pool, no interface
func NewEvalService(pool *pgxpool.Pool) *EvalService { ... }
```

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
- `ResolveProviderConfig()`, `ResolveTag()`

Tests cannot safely run in parallel — they must coordinate env var state with `t.Setenv()` and reset flags.

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

// internal/workflow/preprocess_worker.go:115
cmd := exec.CommandContext(ctx, "git", args...)
cmd.Dir = path
cmd.Run()
```

Git operations are executed directly in worker code, making the `PreprocessWorker.Work()` method impossible to unit test without mocking `exec.Command` (which is notoriously fragile).

### MEDIUM: Missing Interface for Postgres Pool

`*pgxpool.Pool` is used directly everywhere:
- `internal/eval/store.go` — `EvalStore.pool *pgxpool.Pool`
- `internal/api/service_eval.go` — `EvalService.pool *pgxpool.Pool`
- `internal/db/db.go` — `RiverClient.Pool *pgxpool.Pool`

No interface abstraction means all code using these types requires a real Postgres instance to test.

---

## 3. Existing Test Quality Issues

### 3.1 Integration Tests Masked as Unit Tests (need real infra)

| File | Issue |
|------|-------|
| `internal/eval/store_test.go` | All tests require a running Postgres via `connectOrSkip("postgres://...")`. If Postgres is down, tests silently skip. These are **integration tests** living in the wrong place. |
| `internal/workflow/eval_worker_test.go:22` | Test tests nonexistent dataset path — fragile. |
| `internal/workflow/test_helpers_test.go` | `connectOrSkip` requires real Postgres. |
| `internal/workflow/index_worker_test.go` | Tests nonexistent input dir — fragile and doesn't test the actual logic. |
| `internal/store/qdrant_test.go:18,25,32,43,57,97` | **6 tests** using `t.Skip("requires Qdrant server")` — these are not unit tests. |

### 3.2 No Parallel Test Execution

```bash
# No test file calls t.Parallel()
```

A search across all test files shows **zero** calls to `t.Parallel()`. The existing tests:
- Can't safely run in parallel due to global flag/env state
- Don't benefit from multi-core speedup
- Would likely fail under `-parallel` due to shared env vars

### 3.3 Fragile Tests with File System Dependencies

`internal/workflow/index_worker_test.go:24` — `RunIndexing` with a dir that doesn't exist returns a `nil` error:
```go
err := RunIndexing(ctx, IndexArgs{...})
require.Error(t, err)
```
This is testing error behavior for a non-existent directory, but the test is coupled to the filesystem and doesn't test any real indexing logic.

### 3.4 `internal/eval/retrieval_test.go` — Good Example Pattern

This file shows the right approach: mock interfaces (`mockRetriever`, `mockGenerator`), test pure logic, no infrastructure required. More tests should follow this pattern.

---

## 4. Hardcoded Infrastructure Dependencies

| Location | Dependency | Fix |
|----------|-----------|-----|
| `internal/db/db.go:14` | `pgxpool.New(ctx, dsn)` | Accept `pgxpool.Pool` as parameter; let caller connect |
| `internal/db/db.go:30` | `river.NewClient(...)` | Same |
| `internal/store/qdrant.go:29` | `qdrant.NewGrpcClient(...)` | `QdrantStore` should implement `VectorStore`; callers should accept the interface |
| `internal/api/server.go:31` | `db.Connect()` | Accept `*pgxpool.Pool` and `VectorStore` as params instead of creating them |
| `internal/api/service_eval.go:16` | `*pgxpool.Pool` | Define a `EvalStoreInterface` with the needed methods |
| `internal/workflow/eval_worker.go:345` | `createEvalDeps()` — creates real embedder, Qdrant, generator | Accept interfaces instead |
| `internal/workflow/index_worker.go:121` | `RunIndexing()` — creates real embedder, Qdrant, parser, chunker | Same |
| `internal/workflow/preprocess_worker.go:82` | `cloneRepo()` — real git commands | Define `GitRunner` interface |
| `internal/embedder/openai.go:38` | `openai.NewClient(opts...)` | Embedder interface is good; tests use `httptest.Server`. The real issue is that `embedder.New()` creates a real client from env vars. |
| `internal/generator/generator.go:46` | `openai.NewClient(opts...)` | Same pattern as embedder |

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

`internal/workflow/preprocess_worker.go` uses `os.ReadFile`, `filepath.Walk`, `os.Stat`, `os.MkdirAll`, etc. directly. These should be behind filesystem interfaces to enable testing without real files.

### 5.3 Git Command — No Abstraction

`internal/workflow/preprocess_worker.go` uses `exec.CommandContext` for git operations. Should be behind a `GitRunner` interface.

---

## 6. Recommended Actions by Priority

### P0 — Must Fix for Unit Testability (blockers)

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | `internal/config/config.go:37` | Global `flag` in `Load()` | Use private `*flag.FlagSet` parameter; never call `flag.Parse()` in library code |
| 2 | `internal/config/config.go:120-152` | Direct `os.Getenv()` calls | Inject env via a `func(string) string` parameter or accept values directly |
| 3 | `internal/api/server.go:30` | Connects to Postgres/Qdrant in constructor | Accept connections via parameters (dependency injection) |
| 4 | `internal/api/service_eval.go:16` | `*pgxpool.Pool` directly | Define `EvalDB` interface, inject it |
| 5 | `internal/workflow/eval_worker.go:345` | `createEvalDeps()` creates real infra | Accept embedder, Qdrant, generator as parameters |
| 6 | `internal/workflow/index_worker.go:121` | `RunIndexing()` creates real infra | Accept dependencies as parameters |
| 7 | `internal/db/db.go:30` | `NewRiverClient` connects to Postgres | Accept `*pgxpool.Pool` as parameter |

### P1 — Tests Missing for Testable Code (pure logic, no infra needed)

| # | File | Functions | Priority |
|---|------|-----------|----------|
| 1 | `internal/config/normalize.go` | `NormalizeBaseURL`, `WarnOnInsecure`, `ParseRetryAfter` | **HIGH** — pure functions, no deps |
| 2 | `internal/eval/worker.go` | `EvaluateQuestion`, `toFloat32`, `fillRetrievalResult`, `buildContextText`, `generateForQuestion` | **HIGH** — core eval pipeline logic |
| 3 | `internal/workflow/poll_job.go` | `PollUntilTerminal` | **MEDIUM** — needs mock client |
| 4 | `internal/eval/pipeline.go` | `VectorSearcher` interface + `SystemPrompt` | **LOW** — trivial code |

### P2 — Improve Existing Test Quality

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | All test files | No `t.Parallel()` | Add `t.Parallel()` after fixing global state issues |
| 2 | `internal/eval/store_test.go` | Integration test (needs Postgres) | Move to `store_integration_test.go` with build tag or split interfaces |
| 3 | `internal/store/qdrant_test.go` | 6 tests skipped (`t.Skip`) | Move integration tests out; keep unit-testable helpers |
| 4 | `internal/workflow/eval_worker_test.go` | Tests nonexistent dataset | Add proper mock-based tests |
| 5 | `internal/workflow/index_worker_test.go` | Tests non-existent dir | Add proper mock-based tests for `processFile`, `embedAndStore` |

### P3 — Structural Improvements

| # | File | Issue | Fix |
|---|------|-------|-----|
| 1 | `internal/embedder/openai.go:97` | `time.Sleep()` in retry | Make backoff function injectable |
| 2 | `internal/generator/generator.go:84` | `time.Sleep()` in retry | Same |
| 3 | `internal/workflow/preprocess_worker.go:82-124` | Direct git command execution | Define `GitRunner` interface |
| 4 | `internal/workflow/preprocess_worker.go:142-398` | Direct file system calls | Define `FileSystem` interface for walk/read/write |

---

## Summary Statistics

| Metric | Value |
|--------|-------|
| Total non-test Go files | 59 |
| Files with no test file | 27 (46%) |
| Files with inadequate tests | 10+ (integration-only or too trivial) |
| Test files with integration dependencies | 6 (eval/store, workflow/*, store/qdrant) |
| Pure functions untested | 10+ (`normalize.go`, `worker.go`, `poll_job.go`) |
| Tests calling `t.Parallel()` | **0** |
| Uses of `t.Skip("requires ...")` | 6 (all in `qdrant_test.go`) |
| Can run tests without Postgres/Qdrant today | ~70% of test files |

---

*Generated by analyzing 59 source files and 41 test files across 20 packages.*
