# Unit Testability Refactor — Phase-Wise Implementation Plan

> Target: Every test runs in isolation, in parallel, without external infrastructure.
> Progress metric: `go test -count=1 -parallel=8 ./...` passes with zero integration deps.

---

## API-First Plan Impact Summary

The [API-first architecture plan](implementation-plan-api-first.md) has already been partially implemented and significantly affects this refactor:

| Done | Change | Impact on this plan |
|------|--------|---------------------|
| ✅ | `cmd/preprocess/main.go`, `cmd/index/main.go`, `cmd/query/main.go`, `cmd/eval/main.go` deleted | Removed 4 files from the "untested" list — no refactor needed |
| ✅ | `service_workflow.go` — `ResolveTag` calls removed | Simplifies Phase 0.2 (no need to refactor `ResolveTag` in service code) |
| ✅ | `service_chat.go` — Refactored with injectable factory functions | **Phase 4.2 is already done** — just need to add tests |
| ✅ | `types.go` — All fields required, comprehensive `Validate()` methods | Adds new P1 test target (validation logic) |
| ✅ | `index_worker.go` — Fallback defaults removed, `maxIndexFileSize` const | Simplifies Phase 3.2 (fewer code paths) |
| ✅ | `eval_worker.go` — Fallback defaults removed | Simplifies Phase 3.1 (fewer code paths) |
| ❌ | `config.go` — `ResolveTag`, `FloatEnvOrDefault` still present (dead code) | Add simple clean-up to Phase 0 |

---

## Overview

| Phase | Theme | Est. Effort | Rationale |
|-------|-------|-------------|-----------|
| 0 | Foundation: DI wiring + env abstraction | **2 days** | Everything downstream depends on this. Must be first. |
| 1 | Postgres interface extraction | **2 days** | Second most pervasive blocker — used in 3 packages |
| 2 | Pure-function tests (no refactor needed) | **1.5 days** | Low-hanging fruit; includes new validation & refactored ChatService |
| 3 | Worker dependency refactors | **2.5 days** | Eval, index, preprocess workers need interface injection |
| 4 | API layer refactors | **1.5 days** | Server + EvalService constructors (ChatService already done) |
| 5 | Existing test cleanup | **2 days** | Fix integration tests, add t.Parallel(), remove skips |
| 6 | Structural improvements (backoff, git, fs) | **2 days** | Injectable backoff, GitRunner, FileSystem interfaces |
| **Total** | | **~13.5 days** | ~1.5 days saved vs original plan due to API-first work |

---

## Phase 0: Foundation — DI Wiring & Env Abstraction

**Goal:** Make `config.Load()` testable and eliminate global `flag` + `os.Getenv` contamination.

### 0.1 Decouple `config.Load()` from global `flag`

**Files:** `internal/config/config.go`

**Problem:** `flag.IntVar(...)`, `flag.Parse()` use the global `FlagSet`. Any test calling `config.Load()` poisons global flag state for other tests.

| Current code | Issue |
|-------------|-------|
| `flag.IntVar(&cfg.MaxRetries, ...)` | Uses global `flag.CommandLine` |
| `flag.Parse()` | Parses `os.Args[1:]`, contaminates all tests |

**Change:**
```go
type EnvLookup func(key string) string

func LoadWithEnv(flagSet *flag.FlagSet, lookup EnvLookup) (*Config, error) {
    flagSet.IntVar(&cfg.MaxRetries, "max-retries", IntEnvOrDefaultWith(lookup, "MAX_RETRIES", 3), ...)
    flagSet.DurationVar(&cfg.RetryBackoff, "retry-backoff", DurationEnvOrDefaultWith(lookup, "RETRY_BACKOFF", 5*time.Second), ...)
    flagSet.StringVar(&cfg.LogLevel, "log-level", EnvOrDefaultWith(lookup, "LOG_LEVEL", "info"), ...)
    flagSet.Parse(os.Args[1:])  // explicit FlagSet, no side effects
    ...
}

// Backward-compat wrapper for cmd/*/main.go
func Load() (*Config, error) {
    return LoadWithEnv(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Getenv)
}
```

### 0.2 Thread `EnvLookup` through config functions

**Files:** `internal/config/config.go`

**Problem:** `EnvOrDefault`, `IntEnvOrDefault`, `DurationEnvOrDefault`, `FloatEnvOrDefault`, `ResolveProviderConfig` all read `os.Getenv` directly.

**Change:**
```go
// EnvOrDefaultWith — injectable env lookup
func EnvOrDefaultWith(lookup EnvLookup, key, defaultVal string) string {
    if v := lookup(key); v != "" { return v }
    return defaultVal
}

// Backward-compat
func EnvOrDefault(key, defaultVal string) string {
    return EnvOrDefaultWith(os.Getenv, key, defaultVal)
}
```

Do the same for `IntEnvOrDefault`, `DurationEnvOrDefault`, `FloatEnvOrDefault`, `ResolveProviderConfig`.

**Also:** Remove dead code as flagged by the API-first plan:
- Delete `ResolveTag` function (unused since `service_workflow.go` no longer calls it)
- Delete `FloatEnvOrDefault` function (unused after CLI binary deletion)

### 0.3 Add tests for Phase 0 changes

| File | Tests to add |
|------|-------------|
| `internal/config/config_test.go` | `LoadWithEnv` with custom flag + env, parallel-safe, `Config.Validate()` edge cases |
| `internal/config/normalize_test.go` | NEW file: `NormalizeBaseURL`, `ParseRetryAfter`, `WarnOnInsecure` |

---

## Phase 1: Postgres Interface Extraction

**Goal:** Make `EvalStore` and `EvalService` testable without a real Postgres instance.

### 1.1 Define `EvalDB` interface

**Files:** `internal/eval/store.go`

**Change:**
```go
// EvalDB is the interface for eval run storage (enables mocking)
type EvalDB interface {
    CreateRun(ctx context.Context, tag string, strategy map[string]any) (string, error)
    BulkAddQueryResults(ctx context.Context, runID string, results []types.RetrievalResult) error
    UpdateRunMetrics(ctx context.Context, runID string, metrics types.AggregateMetrics) error
    DeleteRunResults(ctx context.Context, runID string) error
    GetRunResults(ctx context.Context, runID string) ([]types.RetrievalResult, error)
}

// Rename existing struct to indicate it's the Postgres impl
type PgEvalStore struct { pool *pgxpool.Pool }

// NewEvalStore returns an EvalDB backed by Postgres
func NewEvalStore(pool *pgxpool.Pool) EvalDB {
    return &PgEvalStore{pool: pool}
}
```

### 1.2 Update `EvalService` to use `EvalDB`

**Files:** `internal/api/service_eval.go`

**Change:**
```go
// BEFORE
type EvalService struct { pool *pgxpool.Pool }

// AFTER
type EvalService struct { store eval.EvalDB }
```

### 1.3 Update `EvalWorker` to use `EvalDB`

**Files:** `internal/workflow/eval_worker.go`

**Change:**
```go
// BEFORE
type EvalWorker struct {
    river.WorkerDefaults[EvalArgs]
    EvalStore *eval.EvalStore               // concrete type
    Client    *river.Client[pgx.Tx]
}

// AFTER
type EvalWorker struct {
    river.WorkerDefaults[EvalArgs]
    EvalDB  eval.EvalDB                      // interface
    Client  *river.Client[pgx.Tx]
}
```

### 1.4 Add mock-based unit tests

| File | Tests to add |
|------|-------------|
| `internal/eval/store_test.go` | Keep integration tests but add unit tests with `mockEvalDB` |
| `internal/api/service_eval_test.go` | NEW file: test `EvalService.ListRuns`, `GetRunSummary`, `GetRun`, `GetRuns` with mock store |

### 1.5 Create `_integration_test.go` pattern

Move existing integration tests behind `//go:build integration`:

```
internal/eval/store_integration_test.go    // needs postgres
internal/workflow/test_helpers_integration_test.go  // needs postgres
internal/store/qdrant_integration_test.go  // needs qdrant
```

---

## Phase 2: Pure-Function Tests (No Refactor Needed)

**Goal:** Cover all pure-logic functions that currently have zero tests.

### 2.1 `internal/eval/worker.go`

**Functions to test:**

| Function | Test cases |
|----------|------------|
| `toFloat32` | empty, normal, rounding |
| `fillRetrievalResult` | hit at rank 1/3, miss, empty results, multiple expected paths, NDCG computation |
| `buildContextText` | single doc, multiple docs, empty |
| `generateForQuestion` | success, API error, empty choices |
| `EvaluateQuestion` | hit, miss, gen error, judge error |

**Pattern:** Use mock `generator.Generator` and `VectorSearcher` (exactly like `retrieval_test.go` does).

### 2.2 `internal/config/normalize.go`

| Function | Test cases |
|----------|------------|
| `NormalizeBaseURL` | has `/v1/`, has `/v1`, no suffix, empty |
| `ParseRetryAfter` | seconds string, RFC1123 date, empty, invalid |
| `WarnOnInsecure` | http + key, https + key, http no key |

### 2.3 `internal/api/types.go` — Validation methods

**NEW — added by API-first plan.** All 4 request types have extensive `Validate()` logic that is currently only tested via handler integration tests.

| Struct | Test cases |
|--------|------------|
| `PreprocessRequest.Validate()` | missing repo_url, missing tag, valid |
| `IndexRequest.Validate()` | missing each field (11 checks), valid |
| `EvalRequest.Validate()` | missing each field (13 checks), valid |
| `ChatRequest.Validate()` | missing each field (9 checks), valid |

### 2.4 `internal/api/service_chat.go` — Refactored by API-first plan

**NOTE:** The API-first plan already refactored `ChatService` with injectable factory functions (`newEmbedderFn`, `newRetrieverFn`, `newGeneratorFn`). No structural refactor needed — just add tests.

| Function | Test cases |
|----------|------------|
| `Chat()` | success with full payload, retriever error, generator error, with memory/conversation |
| `ChatStream()` | success streaming, retriever error, generator error, with memory |
| `buildMessages()` | system prompt, context formatting, memory injection, no memory |
| `retrieveSources()` | successful retrieval, retriever error |

**Pattern:** Use mock factories — override `newEmbedderFn`, `newRetrieverFn`, `newGeneratorFn` to return mock implementations.

### 2.5 `internal/workflow/poll_job.go`

| Function | Test cases |
|----------|------------|
| `PollUntilTerminal` | completes immediately, cancelled, discarded, context timeout, exponential backoff clamping |

**Need:** Mock `JobGetter` interface:
```go
type JobGetter interface {
    JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error)
}
```
Change `PollUntilTerminal` to accept `JobGetter` instead of `*river.Client[...]`.

---

## Phase 3: Worker Dependency Refactors

**Goal:** Make River workers testable without Qdrant, embedder, generator, or git.

### 3.1 `internal/workflow/eval_worker.go`

**Problem:** `EvalWorker.Work()` calls `createEvalDeps()` which connects to real Qdrant and creates real embedder/generator.

**Change — Inject dependencies:**
```go
type EvalWorkerDeps struct {
    EvalDB    eval.EvalDB
    Client    *river.Client[pgx.Tx]
    QStore    eval.VectorSearcher
    Embedder  embedder.Embedder
    Generator generator.Generator
    JudgeGen  generator.Generator
}
```

**Tests:**
- Full pipeline with mock deps: JSONL → embed → search → generate → judge → store
- Checkpoint reloading (delete previous results, skip processed questions)
- Error paths: embed error, search error, generate error

### 3.2 `internal/workflow/index_worker.go`

**Problem:** `RunIndexing()` creates real parser, chunker, embedder, Qdrant connection.

**Change — Accept factory interfaces:**
```go
type IndexDeps struct {
    Parser    parser.Parser
    Chunker   chunker.Chunker
    Embedder  embedder.Embedder
    Store     store.VectorStore
}

func RunIndexing(ctx context.Context, args IndexArgs, deps IndexDeps) error { ... }
```

**Tests:**
- `processFile` with mock parser/chunker/embedder/store
- `embedAndStore` in isolation
- `RunIndexing` — walk a temp dir with test `.md` files

### 3.3 `internal/workflow/preprocess_worker.go`

**Problem:** `cloneRepo` calls `exec.Command("git", ...)`, verify functions use `os.*` directly.

**Change:**
```go
type GitRunner interface {
    Clone(ctx context.Context, url, path string) error
    FetchAll(ctx context.Context, path string) error
    Checkout(ctx context.Context, path, branch string) error
    PullFFOnly(ctx context.Context, path string) error
}

type FileSystem interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm os.FileMode) error
    Walk(root string, fn filepath.WalkFunc) error
    Stat(path string) (os.FileInfo, error)
    MkdirAll(path string, perm os.FileMode) error
}
```

**Tests:**
- `verifyOutput` with in-memory filesystem
- `cloneRepo` with mock `GitRunner`
- `checkFileCountMatch`, `checkDirectoryStructure`, `checkNoShortcodes`, `checkNoRawHTML`, `checkMinimumContent`, `checkTotalSize`

---

## Phase 4: API Layer Refactors

**Goal:** `Server.New()` and `EvalService` testable without infrastructure.

### 4.1 `internal/api/server.go`

**Change — Accept connections instead of creating them:**
```go
func NewWithDeps(cfg *config.Config, pool *pgxpool.Pool, qdrant qstore.VectorStore,
    evalDB eval.EvalDB, chatSvc *ChatService) *Server { ... }
```

**Tests:**
- `NewWithDeps` with mock Qdrant, nil pool → verify routes registered
- `ListenAndServe` — start on random port, send request, verify response

### 4.2 `internal/api/service_eval.go`

Already covered by Phase 1.2 (uses `EvalDB` interface).

**Additional tests:**
- `ListRuns` — pagination, empty results, strategy/metrics JSON parsing
- `GetRunSummary` — found, not found, malformed JSON
- `GetRun` — with questions, without questions, pagination
- `GetRuns` — multiple IDs, some missing

### 4.3 `internal/api/service_chat.go`

**Already refactored by API-first plan.** No structural changes needed. See Phase 2.4 for test targets.

---

## Phase 5: Existing Test Cleanup

**Goal:** Every test file is safe to run under `go test -parallel=8`.

### 5.1 Add `t.Parallel()` everywhere

After Phase 0 removes global `flag` and `os.Getenv` dependencies, add `t.Parallel()` to all test functions in:
- All `internal/*_test.go` files (~39 files)
- `cmd/api/main_test.go` (if exists)
- `cmd/workerd/main_test.go` (if exists)

### 5.2 Fix `internal/eval/store_test.go`

- Rename to `store_integration_test.go` with `//go:build integration`
- Create pure `store_test.go` with mock-based `EvalDB` tests
- Delete `test_helpers_test.go` helpers (moved to integration file)

### 5.3 Fix `internal/workflow/*_test.go`

- `eval_worker_test.go` — Replace `connectOrSkip` + real dataset path with mock deps
- `index_worker_test.go` — Replace non-existent dir test with mock `IndexDeps` tests
- `preprocess_worker_test.go` — Replace with mock Git + FileSystem tests
- `test_helpers_test.go` — Move to `test_helpers_integration_test.go`

### 5.4 Fix `internal/store/qdrant_test.go`

- Rename 6 `t.Skip("requires Qdrant server")` tests to `qdrant_integration_test.go`
- Keep unit-testable: `ToPoint_*`, `ChunkIDToUUID_*`, `isConnError`, `parseDistance`, `storeOnce/searcheOnce/ensureCollectionOnce_notConnected`, `reconnect_NoDSN`, `retry_*`
- Add `t.Parallel()` to all kept tests
- Add DSN parsing tests for `Connect`

### 5.5 Fix handler tests for API-first validation

- `internal/api/handler_workflow_test.go` — Update JSON payloads to include all required fields (Tag, ParserStrategy, TopK, etc.)
- `internal/api/handler_chat_stream_test.go` — Update JSON payloads to include all required fields (Temperature, MaxTokens, LLMProvider, etc.)

---

## Phase 6: Structural Improvements

**Goal:** Eliminate `time.Sleep()` in retry loops, abstract git/filesystem for testability.

### 6.1 Injectable backoff strategy

**Files:** `internal/embedder/openai.go`, `internal/generator/generator.go`

```go
type BackoffStrategy func(attempt int) time.Duration

type embedder struct {
    ...
    backoff BackoffStrategy
}

func newOpenAIEmbedder(baseURL, apiKey, model string, batchSize int, backoff BackoffStrategy) *embedder {
    if backoff == nil {
        backoff = defaultExponentialBackoff
    }
    return &embedder{..., backoff: backoff}
}
```

Tests inject:
```go
backoff: func(attempt int) time.Duration { return 0 }  // no sleep
```

### 6.2 GitRunner interface

Move `internal/workflow/preprocess_worker.go` git helpers to `internal/git/git.go`:
```go
package git

type Runner interface {
    Clone(ctx context.Context, url, path string) error
    FetchAll(ctx context.Context, path string) error
    Checkout(ctx context.Context, path, branch string) error
    PullFFOnly(ctx context.Context, path string) error
}

type OSGit struct{}
// ... wraps exec.CommandContext
```

### 6.3 FileSystem interface

Move verify helpers' filesystem operations behind an interface:
```go
package fs

type Interface interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm os.FileMode) error
    Walk(root string, fn filepath.WalkFunc) error
    Stat(path string) (os.FileInfo, error)
    MkdirAll(path string, perm os.FileMode) error
}
```

---

## Dependency Graph (Execution Order)

```
Phase 0: config.LoadWithEnv, EnvLookup (2 days)
  ↓
Phase 1: EvalDB interface (2 days) ←──────────┐
  ↓                                             │
Phase 3: Worker dep refactors (2.5 days)       │  Phase 2: Pure-function tests (1.5 days)
  (depends on EvalDB, embedder/                │  (independent — can run in
   generator interfaces)                       │   parallel with Phase 1)
  ↓                                             │
Phase 4: API layer refactors (1.5 days) ───────┤
  ↓                                             │
Phase 5: Test cleanup (2 days) ─────────────────┤  (depends on Phase 0 for
  ↓                                             │   t.Parallel safety)
Phase 6: Structural improvements (2 days) ──────┘
  (independent)
```

---

## Test Coverage Target After Each Phase

| Phase | Coverage (approx) | Running `go test -parallel=8 ./...` |
|-------|-------------------|-------------------------------------|
| Start | ~45% | Fails — needs Postgres/Qdrant |
| 0     | 45% | Fails — infura deps remain |
| 1     | 55% | **Passes** `./internal/eval/...` without Postgres |
| 2     | 65% | Passes most internal packages |
| 3     | 75% | Workers testable without infra |
| 4     | 85% | Full API layer testable |
| 5     | 85% | No skipped tests, all parallel |
| 6     | 88% | Backoff/fakes improved, sleep removed from tests |

---

## Key Design Principles

1. **Backward compatibility:** All `cmd/*/main.go` files continue working unchanged. New constructors/loaders coexist with old ones.
2. **No external dependencies in tests:** Mock interfaces define exactly the contract needed.
3. **Table-driven tests:** Follow the existing good patterns in `internal/chunker/fixed_test.go` and `internal/eval/metrics_test.go`.
4. **One change per PR:** Each phase is independently reviewable and mergable.
5. **Build tags for integration:** `//go:build integration` prevents accidental execution in CI unit test runs.

---

## CI Changes

Add to `make.cmd`:
```makefile
test-unit:
    go test -count=1 -parallel=8 -tags=unit -coverprofile=coverage-unit.out ./...
    go tool cover -func=coverage-unit.out

test-all:
    go test -count=1 -parallel=8 -tags="unit integration" -coverprofile=coverage-all.out ./...
```

Update AGENTS.md to document the new patterns.
