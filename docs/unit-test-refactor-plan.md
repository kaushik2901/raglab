# Unit Testability Refactor — Phase-Wise Implementation Plan

> Target: Every test runs in isolation, in parallel, without external infrastructure.
> Progress metric: `go test -count=1 -parallel=8 ./...` passes with zero integration deps.

---

## Overview

| Phase | Theme | Est. Effort | Rationale |
|-------|-------|-------------|-----------|
| 0 | Foundation: DI wiring + env abstraction | **3 days** | Everything downstream depends on this. Must be first. |
| 1 | Postgres interface extraction | **2 days** | Second most pervasive blocker — used in 3 packages |
| 2 | Pure-function tests (no refactor needed) | **1 day** | Low-hanging fruit, can be done in parallel with P1 |
| 3 | Worker dependency refactors | **3 days** | Eval, index, preprocess workers need interface injection |
| 4 | API layer refactors | **2 days** | Server, ChatService, EvalService constructors |
| 5 | Existing test cleanup | **2 days** | Fix integration tests, add t.Parallel(), remove skips |
| 6 | Structural improvements (backoff, git, fs) | **2 days** | Injectable backoff, GitRunner, FileSystem interfaces |
| **Total** | | **~15 days** | |

---

## Phase 0: Foundation — DI Wiring & Env Abstraction

**Goal:** Make `config.Load()` testable and eliminate global `flag` + `os.Getenv` contamination.

### 0.1 Decouple `config.Load()` from global `flag`

**Files:** `internal/config/config.go`

**Problem:** `flag.IntVar(...)`, `flag.Parse()` use the global `FlagSet`. Any test calling `config.Load()` poisons global flag state for other tests.

**Change:**
```go
// BEFORE
func Load() (*Config, error) {
    flag.IntVar(&cfg.MaxRetries, "max-retries", ...)
    flag.Parse()
    ...
}

// AFTER
type EnvLookup func(key string) string

func LoadWithEnv(flagSet *flag.FlagSet, lookup EnvLookup) (*Config, error) {
    flagSet.IntVar(&cfg.MaxRetries, "max-retries", ...)
    ...
    flagSet.Parse(os.Args[1:])  // explicit FlagSet, no side effects
    ...
}

// Backward-compat wrapper for cmd/*
func Load() (*Config, error) {
    return LoadWithEnv(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Getenv)
}
```

### 0.2 Thread `EnvLookup` through downstream packages

**Files:** `internal/config/config.go`, `internal/config/normalize.go`

**Problem:** `EnvOrDefault`, `ResolveProviderConfig`, `ResolveTag` all read `os.Getenv` directly.

**Change:**
```go
// BEFORE: os.Getenv() hard-coded
func EnvOrDefault(key, defaultVal string) string {
    if v := os.Getenv(key); v != "" { return v }
    return defaultVal
}

// AFTER: Accept lookup function
func EnvOrDefaultWith(lookup EnvLookup, key, defaultVal string) string {
    if v := lookup(key); v != "" { return v }
    return defaultVal
}

// Backward-compat
func EnvOrDefault(key, defaultVal string) string {
    return EnvOrDefaultWith(os.Getenv, key, defaultVal)
}
```

Do the same for `IntEnvOrDefault`, `DurationEnvOrDefault`, `FloatEnvOrDefault`, `ResolveProviderConfig`, `ResolveTag`.

### 0.3 Extract `EmbedderFactory` and `GeneratorFactory`

**Files:** `internal/embedder/embedder.go`, `internal/generator/generator.go`

**Problem:** `embedder.New()` and `generator.New()` read env vars internally.

**Change:**
```go
type EmbedderFactory func(provider config.Provider, model string, batchSize int) (Embedder, error)

func NewWithLookup(lookup config.EnvLookup, provider config.Provider, ...) (Embedder, error) {
    baseURL, apiKey := config.ResolveProviderConfigWith(lookup, provider)
    ...
}

// cmd/* use os.Getenv, tests inject fake lookup
var embedderNew = embedder.New  // ← overridable in tests
```

### 0.4 Add tests for everything in Phase 0

| File | Tests to add |
|------|-------------|
| `internal/config/config_test.go` | `LoadWithEnv` with custom flag + env, `ResolveTag` with injected time, parallel-safe |
| `internal/config/normalize_test.go` | `NormalizeBaseURL`, `ParseRetryAfter`, `WarnOnInsecure` (NEW file) |
| `internal/embedder/factory_test.go` | `NewWithLookup` with fake env |
| `internal/generator/factory_test.go` | Same |

---

## Phase 1: Postgres Interface Extraction

**Goal:** Make `EvalStore` and `EvalService` testable without a real Postgres instance.

### 1.1 Define `EvalDB` interface

**Files:** `internal/eval/store.go`

**Change:**
```go
// internal/eval/store.go

type EvalDB interface {
    CreateRun(ctx context.Context, tag string, strategy map[string]any) (string, error)
    BulkAddQueryResults(ctx context.Context, runID string, results []types.RetrievalResult) error
    UpdateRunMetrics(ctx context.Context, runID string, metrics types.AggregateMetrics) error
    DeleteRunResults(ctx context.Context, runID string) error
    GetRunResults(ctx context.Context, runID string) ([]types.RetrievalResult, error)
}

// Rename existing struct to indicate it's the Postgres impl
type PgEvalStore struct {
    pool *pgxpool.Pool
}

// Constructor now returns EvalDB, not *EvalStore
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
func NewEvalService(pool *pgxpool.Pool) *EvalService { ... }

// AFTER
type EvalService struct { store eval.EvalDB }
func NewEvalService(store eval.EvalDB) *EvalService { ... }
```

### 1.3 Add mock-based tests

| File | Tests to add |
|------|-------------|
| `internal/eval/store_test.go` | Keep integration tests but add `_test.go` unit tests with `mockEvalDB` — test `EvalService.ListRuns`, `GetRunSummary`, etc. via mocked store |
| `internal/api/service_eval_test.go` | NEW file: test `EvalService` with `mockEvalDB` |

### 1.4 Create `_integration_test.go` pattern

Move existing integration tests to `store_integration_test.go` with `//go:build integration` build tag:

```
internal/eval/store_integration_test.go  // needs postgres
internal/workflow/test_helpers_integration_test.go  // needs postgres
internal/store/qdrant_integration_test.go  // needs qdrant
```

Add to `make.cmd`:
```makefile
test-unit:
    go test -count=1 -tags=unit ./...

test-integration:
    go test -count=1 -tags=integration ./...
```

---

## Phase 2: Pure-Function Tests (No Refactor Needed)

**Goal:** Cover all pure-logic functions that currently have zero tests.

### 2.1 `internal/eval/worker.go`

**Functions to test:**

| Function | Input | Output | Test cases |
|----------|-------|--------|------------|
| `toFloat32` | `[]float64` | `[]float32` | empty, normal, rounding |
| `fillRetrievalResult` | `EvalQuestion` + search results + topK | populated `RetrievalResult` | hit at rank 1, hit at rank 3, miss, empty results, multiple expected paths, NDCG computation |
| `buildContextText` | search results | formatted string | single doc, multiple docs, empty |
| `generateForQuestion` | context + generator | answer + tokens | success, API error, empty choices |
| `EvaluateQuestion` | full eval pipeline step | `RetrievalResult` | hit, miss, gen error, judge error |

**Pattern:** Use mock `generator.Generator` and `VectorSearcher` (exactly like `retrieval_test.go` does).

**Estimated tests:** ~20 table-driven tests.

### 2.2 `internal/config/normalize.go`

| Function | Test cases |
|----------|------------|
| `NormalizeBaseURL` | has `/v1/`, has `/v1`, no suffix, empty |
| `ParseRetryAfter` | seconds string, RFC1123 date, empty, invalid |
| `WarnOnInsecure` | http + key, https + key, http no key |

### 2.3 `internal/workflow/poll_job.go`

| Function | Test cases |
|----------|------------|
| `PollUntilTerminal` | completes immediately, cancelled, discarded, context timeout, exponential backoff clamping |

**Need:** Mock `*river.Client[pgx.Tx]` via the `JobGet` method. Define a small interface:

```go
type JobGetter interface {
    JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error)
}
```

Change `PollUntilTerminal` to accept `JobGetter` instead of `*river.Client[...]`.

### 2.4 `internal/types/` (remaining structs)

Add creation tests for:
- `Document` — zero value, populated
- `EvalQuestion` — JSON round-trip (uses json tags)
- `RelevanceJudgment` — JSON round-trip
- `AggregateMetrics` — zero value

---

## Phase 3: Worker Dependency Refactors

**Goal:** Make River workers testable without Qdrant, embedder, generator, or git.

### 3.1 `internal/workflow/eval_worker.go`

**Problem:** `EvalWorker.Work()` calls `createEvalDeps()` which:
1. Reads `os.Getenv("QDRANT_URL")`, `os.Getenv("QDRANT_API_KEY")`
2. Creates real `embedder.New(...)`, `qstore.NewQdrantStore(...).Connect(...)`, `generator.New(...)` × 2

**Change — Inject dependencies:**

```go
type EvalWorkerDeps struct {
    EvalStore eval.EvalDB
    Client    *river.Client[pgx.Tx]
    QStore    eval.VectorSearcher
    Embedder  embedder.Embedder
    Generator generator.Generator
    JudgeGen  generator.Generator
}

type EvalWorker struct {
    river.WorkerDefaults[EvalArgs]
    Deps EvalWorkerDeps
}
```

Create a `NewEvalWorker` that takes the deps struct. The `Work()` method no longer calls `createEvalDeps()`.

**Tests:**
- `EvalWorker` with mock embedder, mock Qdrant, mock generators, mock `EvalDB`
- Test the pipeline: JSONL → embed → search → generate → judge → store
- Test checkpoint reloading (delete previous results, skip processed questions)
- Test error paths: embed error, search error, generate error

### 3.2 `internal/workflow/index_worker.go`

**Problem:** `RunIndexing()` creates real parser, chunker, embedder, Qdrant connection.

**Change — Accept factory interfaces:**

```go
type IndexDeps struct {
    Parser  parser.Parser
    Chunker chunker.Chunker
    Embedder embedder.Embedder
    Store   store.VectorStore
}

func RunIndexing(ctx context.Context, args IndexArgs, deps IndexDeps) error { ... }
```

**Tests:**
- `processFile` with mock parser emitting known elements, mock chunker, mock embedder, mock store
- `embedAndStore` in isolation
- `RunIndexing` — walk a temp dir with test `.md` files, verify store receives correct chunks

### 3.3 `internal/workflow/preprocess_worker.go`

**Problem:** `cloneRepo` calls `exec.Command("git", ...)`, verify functions call `os.ReadFile`, `filepath.Walk`.

**Change — Split into two parts:**

**Part A: Git abstraction**
```go
type GitRunner interface {
    Clone(ctx context.Context, url, path string) error
    Fetch(ctx context.Context, path string) error
    Checkout(ctx context.Context, path, branch string) error
    Pull(ctx context.Context, path string) error
}
```

**Part B: File system abstraction**
```go
type FileSystem interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte) error
    Walk(root string, fn filepath.WalkFunc) error
    Stat(path string) (os.FileInfo, error)
    MkdirAll(path string) error
}
```

**Change constructor:**
```go
type PreprocessWorker struct {
    river.WorkerDefaults[PreprocessArgs]
    Client *river.Client[pgx.Tx]
    Git    GitRunner
    FS     FileSystem
}
```

**Tests:**
- `verifyOutput` with in-memory filesystem
- `cloneRepo` with mock `GitRunner`
- `checkFileCountMatch`, `checkDirectoryStructure`, `checkNoShortcodes`, `checkNoRawHTML`, `checkMinimumContent`, `checkTotalSize` — all testable with fake `FileSystem`

---

## Phase 4: API Layer Refactors

**Goal:** `Server.New()`, `ChatService.NewChatService()`, `EvalService` testable without infrastructure.

### 4.1 `internal/api/server.go`

**Change — Accept connections instead of creating them:**

```go
func NewWithDeps(cfg *config.Config, pool *pgxpool.Pool, qdrant qstore.VectorStore) *Server { ... }

// Old New() kept for backward compat in cmd/api/main.go
func New(cfg *config.Config) (*Server, error) {
    pool, err := db.Connect(context.Background(), cfg.DatabaseURL)
    ...
    qdrant := qstore.NewQdrantStore(cfg.QdrantAPIKey)
    qdrant.Connect(context.Background(), cfg.QdrantURL)
    return NewWithDeps(cfg, pool, qdrant), nil
}
```

**Tests:**
- `NewWithDeps` with mock Qdrant, nil pool → verify routes registered
- `ListenAndServe` — start on random port, send request, verify response

### 4.2 `internal/api/service_chat.go`

**Change — Accept ready-made dependencies:**

```go
type ChatServiceDeps struct {
    Embedder  embedder.Embedder
    Retriever retrieverInterface
    Generator generator.Generator
    Memory    memory.Memory
}

func NewChatServiceWithDeps(deps ChatServiceDeps) *ChatService { ... }

// Old NewChatService rewritten to use factory
func NewChatService(cfg *config.Config, vs qstore.VectorStore) (*ChatService, error) {
    emb, _ := embedder.New(...)
    ret, _ := retriever.New(emb, vs, "naive-search")
    gen, _ := generator.New(...)
    return NewChatServiceWithDeps(ChatServiceDeps{
        Embedder: emb, Retriever: ret, Generator: gen,
        Memory: memory.NewRingBuffer(cfg.ChatMemorySize),
    }), nil
}
```

**Tests:**
- `Chat` with mock retriever + mock generator → verify answer, sources, token usage
- `Chat` with conversation ID → verify memory stores turns
- `Chat` with retriever error → verify error propagation
- `retrieveSources` with mock retriever
- `buildMessages` — verify system prompt, context formatting, memory injection
- `resolveMaxTokens`, `resolveTemperature` — edge cases

### 4.3 `internal/api/service_eval.go`

Already covered in Phase 1 (uses `EvalDB` interface). Add tests for:
- `ListRuns` — pagination, empty results, parsing strategy/metrics JSON
- `GetRunSummary` — found, not found, malformed JSON
- `GetRun` — with questions, without questions, pagination
- `GetRuns` — multiple IDs, missing IDs

---

## Phase 5: Existing Test Cleanup

**Goal:** Every test file is safe to run under `go test -parallel=8`.

### 5.1 Add `t.Parallel()` everywhere

After Phase 0 removes global `flag` and `os.Getenv` dependencies, add:

```go
func TestSomething(t *testing.T) {
    t.Parallel()
    ...
}
```

**Files:**
- All `internal/*_test.go` files (41 files)
- Both `cmd/*/main_test.go` files

**Exceptions:** Tests that use shared resources (e.g., same temp dir pattern) must be reviewed.

### 5.2 Fix `internal/eval/store_test.go`

**Changes:**
- Rename to `store_integration_test.go` with `//go:build integration`
- Create pure `store_test.go` with mock-based tests for `EvalDB` interface behavior

### 5.3 Fix `internal/workflow/*_test.go`

**Changes:**
- `eval_worker_test.go` — Replace `connectOrSkip` + real dataset path with mock-based tests
- `index_worker_test.go` — Replace temp dir + error assertion with mock parser/chunker/embedder/store
- `preprocess_worker_test.go` — Replace with mock Git + FileSystem tests
- `test_helpers_test.go` — Move to `test_helpers_integration_test.go`

### 5.4 Fix `internal/store/qdrant_test.go`

**Changes:**
- Rename integration tests to `qdrant_integration_test.go` with build tags
- Keep unit-testable tests: `ToPoint_*`, `ChunkIDToUUID_*`, `isConnError`, `parseDistance`, `storeOnce/searcheOnce/ensureCollectionOnce_notConnected`, `reconnect_NoDSN`, `retry_*`
- Add `t.Parallel()` to all kept tests
- Add tests for `Connect` DSN parsing (host, port, TLS)

---

## Phase 6: Structural Improvements

**Goal:** Eliminate `time.Sleep()` in retry loops, abstract git/filesystem for testability.

### 6.1 Injectable backoff strategy

**Files:** `internal/embedder/openai.go`, `internal/generator/generator.go`

**Change:**
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

Default implementation uses `time.Sleep`. Tests inject:
```go
backoff: func(attempt int) time.Duration { return 0 }  // no sleep
```

### 6.2 HttpClient abstraction for embedder/generator

**Alternative to 6.1:** Since both already use `openai.Client` (which is configurable via `option.WithHTTPClient`), tests can inject a custom transport. This is already done via `httptest.NewServer` in tests. The real issue is that the factory (`embedder.New`, `generator.New`) creates a real client pointing to real servers.

**Refined approach:** Keep 6.1 (backoff injection) and ensure tests can inject a mock http round-tripper.

### 6.3 GitRunner interface

**Files:** `internal/workflow/preprocess_worker.go`

**Change:** (Already described in Phase 3.3)

Add `internal/git/git.go`:
```go
package git

type Runner interface {
    Clone(ctx context.Context, url, path string, opts ...string) error
    FetchAll(ctx context.Context, path string) error
    Checkout(ctx context.Context, path, branch string) error
    PullFFOnly(ctx context.Context, path string) error
}

type OSGit struct{}

func (OSGit) Clone(ctx context.Context, url, path string, opts ...string) error {
    return exec.CommandContext(ctx, "git", append([]string{"clone", "--depth", "1", url, path}, opts...)...).Run()
}
```

### 6.4 FileSystem interface

**Files:** `internal/workflow/preprocess_worker.go` (verify helpers)

Create `internal/fs/fs.go`:
```go
package fs

type Interface interface {
    ReadFile(path string) ([]byte, error)
    WriteFile(path string, data []byte, perm os.FileMode) error
    Walk(root string, fn filepath.WalkFunc) error
    Stat(path string) (os.FileInfo, error)
    MkdirAll(path string, perm os.FileMode) error
}

type OS struct{}

func (OS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
// ...
```

---

## Dependency Graph (Execution Order)

```
Phase 0: config.LoadWithEnv, EnvLookup
  ↓
Phase 1: EvalDB interface ←──────┐
  ↓                                │
Phase 3: Worker dep refactors     │  Phase 2: Pure-function tests
  (depend on EvalDB, embedder/    │  (independent — can run in
   generator interfaces)           │   parallel with Phase 1)
  ↓                                │
Phase 4: API layer refactors ──────┤  Phase 5: Test cleanup
  (depends on EvalDB, embedder/    │  (depends on Phase 0 for
   generator, VectorStore)         │   t.Parallel safety)
  ↓                                │
Phase 6: Structural improvements ──┘  Phase 6: Backoff, Git, FS
  (independent)                        (independent)
```

---

## Test Coverage Target After Each Phase

| Phase | Coverage (approx) | Running `go test -parallel=8 ./...` |
|-------|-------------------|-------------------------------------|
| Start | ~45% of packages | Fails — needs Postgres/Qdrant |
| 0     | 45% | Still fails — global flag fixed but infra deps remain |
| 1     | 55% | **Passes** `./internal/eval/...` without Postgres |
| 2     | 65% | Passes most internal packages |
| 3     | 75% | Workers testable without infra |
| 4     | 85% | Full API layer testable |
| 5     | 85% | No skipped tests, all parallel |
| 6     | 88% | Backoff/fakes improved, sleep removed from tests |

---

## Key Design Principles

1. **Backward compatibility:** All `cmd/*/main.go` files continue working unchanged. New constructors/loaders coexist with old ones.
2. **No external dependencies in tests:** Mock interfaces define exactly the contract needed. No testify mocks forced — use them where appropriate.
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
