# Phase 1: River + Pipeline Workers — Implementation Plan

## Overview

Wrap the existing preprocessing and indexing CLI pipelines as durable River workers backed by Postgres. The CLIs become thin wrappers that insert a River job and tail progress.

---

## Sub-Phases

### Phase 1.1: Infrastructure — Postgres + River

**Goal:** Working Postgres instance and River client that can enqueue and process jobs.

#### Docker Compose

Add Postgres to `docker-compose.yml`:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: rag
      POSTGRES_PASSWORD: rag
      POSTGRES_DB: rag
    ports:
      - "5432:5432"
    volumes:
      - pg_data:/var/lib/postgresql/data

  qdrant:
    # ... existing, unchanged ...
```

#### Dependencies

```
go get github.com/riverqueue/river@v0.14.0
go get github.com/jackc/pgx/v5@latest
```

Pin a specific River version to avoid unexpected breaking changes.

#### Database Connection

New package `internal/db/db.go` — shared PG connection pool:

```go
package db

import (
    "context"
    "fmt"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        dsn = "postgres://rag:rag@localhost:5432/rag?sslmode=disable"
    }
    pool, err := pgxpool.New(ctx, dsn)
    if err != nil {
        return nil, fmt.Errorf("connect to postgres: %w", err)
    }
    return pool, nil
}
```

#### Schema Migrations

New directory `internal/db/migrations/` with a runner (`migrate.go`):

```sql
-- 001_create_workflow_tables.sql

CREATE TABLE IF NOT EXISTS workflows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type        TEXT NOT NULL,              -- 'preprocess' | 'index'
    tag         TEXT NOT NULL,              -- user-specified or auto-generated
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | running | succeeded | failed
    input_params JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflows_tag ON workflows(tag);
CREATE INDEX idx_workflows_type ON workflows(type);
CREATE INDEX idx_workflows_status ON workflows(status);

CREATE TABLE IF NOT EXISTS workflow_steps (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    step_name   TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'pending',  -- pending | running | succeeded | failed
    attempts    INT NOT NULL DEFAULT 0,
    error       TEXT,
    output      JSONB,
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_workflow_steps_wf ON workflow_steps(workflow_id);

-- River creates its own internal tables automatically via river.NewClient
```

River's `NewClient` auto-creates `river_job` and related tables on first initialization.

#### Config Changes

Extend `internal/config/config.go`:

```go
type Config struct {
    // ... existing fields ...

    DatabaseURL string  // DATABASE_URL env var
    Tag         string  // --tag / TAG (optional, auto-generated)
    InputTag    string  // --input-tag / INPUT_TAG (for indexing workflows)
}
```

Add parsing:

```go
cfg.DatabaseURL = envOrDefault("DATABASE_URL", "postgres://rag:rag@localhost:5432/rag?sslmode=disable")

var tag string
flag.StringVar(&tag, "tag", "", "Workflow tag (auto-generated if empty)")
// After flag.Parse:
if tag != "" {
    cfg.Tag = tag
} else {
    cfg.Tag = autoGenerateTag() // e.g. "pre-20260603-143022"
}

var inputTag string
flag.StringVar(&inputTag, "input-tag", "", "Source preprocessed tag (for indexing)")
cfg.InputTag = inputTag
```

**Files:**
- `internal/db/db.go`
- `internal/db/migrate.go`
- `internal/db/migrations/001_create_workflow_tables.sql`
- `docker-compose.yml` (modified)

**Deliverables:**
- `docker compose up` starts Postgres on 5432
- Code can connect and run migrations

---

### Phase 1.2: Workflow DB Layer

**Goal:** CRUD operations for workflows and workflow_steps tables, reusable by all workers.

New package `internal/workflow/store.go`:

```go
package workflow

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Store struct {
    pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store

func (s *Store) CreateWorkflow(ctx, wfType, tag string, params map[string]any) (workflowID string, err error)
func (s *Store) UpdateWorkflowStatus(ctx, workflowID, status string) error
func (s *Store) GetWorkflow(ctx, workflowID string) (*types.Workflow, error)
func (s *Store) ListWorkflows(ctx, wfType, tag, status string, limit, offset int) ([]types.Workflow, error)

func (s *Store) CreateStep(ctx, workflowID, stepName string) (stepID string, err error)
func (s *Store) UpdateStepStatus(ctx, stepID, status string, errMsg *string, output map[string]any) error
func (s *Store) GetSteps(ctx, workflowID string) ([]types.WorkflowStep, error)

// LoadState assembles the state map from the workflow's input_params
// and all completed step outputs (for passing to existing pipeline.Stage.Run)
func (s *Store) LoadState(ctx, workflowID string) (map[string]any, error)
```

New types in `internal/types/workflow.go`:

```go
type Workflow struct {
    ID          string
    Type        string
    Tag         string
    Status      string
    InputParams map[string]any
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type WorkflowStep struct {
    ID          string
    WorkflowID  string
    StepName    string
    Status      string
    Attempts    int
    Error       *string
    Output      map[string]any
    StartedAt   *time.Time
    CompletedAt *time.Time
}
```

**Files:**
- `internal/workflow/store.go`
- `internal/types/workflow.go`

**Deliverables:**
- `Store.CreateWorkflow` + `GetWorkflow` tested with a real PG or `pgtap`
- `Store.LoadState` correctly merges input params + step outputs

---

### Phase 1.3: River Workers — Preprocessing

**Goal:** Clone → Preprocess → Verify as a linear chain of River workers.

#### Job Args

```go
// internal/workflow/preprocess_worker.go

type CloneArgs struct {
    WorkflowID string `json:"workflow_id"`
    Tag        string `json:"tag"`
    RepoURL    string `json:"repo_url"`
    RepoPath   string `json:"repo_path"`
}

type PreprocessArgs struct {
    WorkflowID  string   `json:"workflow_id"`
    Tag         string   `json:"tag"`
    RepoPath    string   `json:"repo_path"`    // artifacts/preprocessing/{tag}/repo/
    OutputPath  string   `json:"output_path"`  // artifacts/preprocessing/{tag}/output/
    IncludeDirs []string `json:"include_dirs,omitempty"`
}

type VerifyArgs struct {
    WorkflowID string `json:"workflow_id"`
    Tag        string `json:"tag"`
    RepoPath   string `json:"repo_path"`   // artifacts/preprocessing/{tag}/repo/
    OutputPath string `json:"output_path"` // artifacts/preprocessing/{tag}/output/
}

func (CloneArgs) Kind() string     { return "clone" }
func (PreprocessArgs) Kind() string { return "preprocess" }
func (VerifyArgs) Kind() string     { return "verify" }
```

#### Worker Pattern

Each worker follows the same structure:

```go
type CloneWorker struct {
    store *workflow.Store
    river.Client
}

func (w *CloneWorker) Work(ctx context.Context, job *river.Job[CloneArgs]) error {
    // 1. Mark step as running
    step := w.store.CreateStep(ctx, job.Args.WorkflowID, "clone")

    // 2. Build a temporary config from args
    cfg := &config.Config{
        RepoURL:  job.Args.RepoURL,
        RepoPath: job.Args.RepoPath,
    }

    // 3. Load workflow state (previous step outputs)
    state, err := w.store.LoadState(ctx, job.Args.WorkflowID)
    if err != nil {
        return fmt.Errorf("load state: %w", err)
    }

    // 4. Call existing stage logic
    stage := stagepkg.CloneStage(cfg)
    result, err := stage.Run(ctx, state)
    if err != nil {
        w.store.UpdateStepStatus(ctx, step.ID, "failed", &errStr, nil)
        return err // River retries
    }

    // 5. Save step output
    w.store.UpdateStepStatus(ctx, step.ID, "succeeded", nil, result.Output)

    // 6. Enqueue next step with paths derived from tag
    repoPath := filepath.Join("artifacts", "preprocessing", job.Args.Tag, "repo")
    outPath := filepath.Join("artifacts", "preprocessing", job.Args.Tag, "output")
    _, err = w.Client.Insert(ctx, &PreprocessArgs{
        WorkflowID:  job.Args.WorkflowID,
        Tag:         job.Args.Tag,
        RepoPath:    repoPath,
        OutputPath:  outPath,
        IncludeDirs: job.Args.IncludeDirs,
    })
    return err
}
```

#### Worker Registration

```go
// cmd/workerd/main.go — or integrated into cmd/preprocess/main.go
workers := river.NewWorkers()
river.AddWorker(workers, &CloneWorker{store, client})
river.AddWorker(workers, &PreprocessWorker{store, client})
river.AddWorker(workers, &VerifyWorker{store, client})

// Start River
queues := &river.QueueConfig{MaxWorkers: 5}
riverClient, _ := river.NewClient(pool, &river.Config{
    Queues:  map[string]*river.QueueConfig{"default": queues},
    Workers: workers,
})
```

**Key design decisions:**

1. **Output dir**: all paths are derived from the workflow type and tag:
   - Cloned repo: `artifacts/preprocessing/{tag}/repo/`
   - Preprocessed output: `artifacts/preprocessing/{tag}/output/`
   - This ensures tag-based isolation and a clean, predictable artifact tree.
2. **Error handling**: Return `err` from `Work()` for River's built-in retry with backoff. Only mark step as failed when retries exhausted.
3. **State reconstruction**: `store.LoadState()` reads all completed steps' outputs and merges them, so each worker sees the accumulated state (same as the sequential CLI runner).

**Files:**
- `internal/workflow/preprocess_worker.go` — all 3 workers + args
- `cmd/workerd/main.go` or update `cmd/preprocess/main.go`

**Deliverables:**
- `CloneWorker` + `PreprocessWorker` + `VerifyWorker` compile and pass unit tests
- `store.LoadState` correctly reproduces the `state` map that existing stages expect

---

### Phase 1.4: River Workers — Indexing

**Goal:** Parse → Chunk → Embed → Store as River workers.

Same pattern as preprocessing. Job args carry indexing params:

```go
type ParseArgs struct {
    WorkflowID string `json:"workflow_id"`
    Tag        string `json:"tag"`        // this index run's tag (e.g. "idx-v2")
    InputTag   string `json:"input_tag"`  // source preprocessed tag (e.g. "pre-v2")
}

type ChunkArgs struct {
    WorkflowID    string `json:"workflow_id"`
    Tag           string `json:"tag"`
    InputTag      string `json:"input_tag"`
    ChunkStrategy string `json:"chunk_strategy"`
    ChunkSize     int    `json:"chunk_size"`
    ChunkOverlap  int    `json:"chunk_overlap"`
}

type EmbedArgs struct {
    WorkflowID     string `json:"workflow_id"`
    Tag            string `json:"tag"`
    InputTag       string `json:"input_tag"`
    EmbeddingModel string `json:"embedding_model"`
    BatchSize      int    `json:"batch_size"`
    LLMBaseURL     string `json:"llm_base_url"`
    LLMApiKey      string `json:"llm_api_key"`
}

type StoreArgs struct {
    WorkflowID    string `json:"workflow_id"`
    Tag           string `json:"tag"`
    InputTag      string `json:"input_tag"`
    QdrantURL     string `json:"qdrant_url"`
    QdrantAPIKey  string `json:"qdrant_api_key"`
}
```

**Critical difference from preprocessing:** The `StoreArg` determines the Qdrant collection name from `Tag`:

```go
// In StoreWorker:
collectionName := job.Args.Tag   // e.g. "idx-v2"
qStore.EnsureCollection(ctx, collectionName, vectorSize, "Cosine")
```

**Input path resolution:** The indexing workers read from the upstream preprocessing tag's output:

```go
// In ParseWorker:
inputPath := filepath.Join("artifacts", "preprocessing", job.Args.InputTag, "output")
// walks this directory for .md files instead of cfg.OutputPath
```

**Files:**
- `internal/workflow/index_worker.go` — all 4 workers + args

**Deliverables:**
- All 4 indexing workers compile and pass unit tests
- Qdrant collection named after the tag

---

### Phase 1.5: CLI Wrappers

**Goal:** Existing CLIs become thin wrappers that insert River jobs.

#### `cmd/preprocess/main.go` (rewritten)

```go
func run(args []string) error {
    cfg := parseConfig(args) // existing config parsing, now includes --tag

    pool, _ := db.Connect(ctx)
    defer pool.Close()

    store := workflow.NewStore(pool)
    wfID, _ := store.CreateWorkflow(ctx, "preprocess", cfg.Tag, map[string]any{
        "repo_url":  cfg.RepoURL,
        "repo_path": cfg.RepoPath,
        "include_dirs": cfg.IncludeDirs,
    })

    riverClient, _ := river.NewClient(pool, &river.Config{...})
    _, _ = riverClient.Insert(ctx, &CloneArgs{
        WorkflowID: wfID,
        Tag:        cfg.Tag,
        RepoURL:    cfg.RepoURL,
        RepoPath:   cfg.RepoPath,
    })

    // Poll workflow status until completion (or exit immediately with --async)
    return pollUntilDone(ctx, store, wfID)
}
```

#### `cmd/index/main.go` (rewritten)

Same pattern, but creates an "index" type workflow and inserts a `ParseArgs` job. Accepts `--tag` and `--input-tag`.

#### Shared polling helper

```go
// internal/workflow/poll.go
func PollUntilDone(ctx context.Context, store *Store, wfID string, interval time.Duration) error {
    for {
        wf, _ := store.GetWorkflow(ctx, wfID)
        switch wf.Status {
        case "succeeded":
            return nil
        case "failed":
            steps, _ := store.GetSteps(ctx, wfID)
            for _, s := range steps {
                if s.Status == "failed" {
                    return fmt.Errorf("step %s failed: %s", s.StepName, *s.Error)
                }
            }
            return fmt.Errorf("workflow %s failed", wfID)
        }
        time.Sleep(interval)
    }
}
```

**Files:**
- `cmd/preprocess/main.go` (rewritten)
- `cmd/index/main.go` (rewritten)
- `internal/workflow/poll.go`

**Deliverables:**
- `bin\preprocess.exe --tag pre-v2` inserts a River job and waits
- `bin\index.exe --tag idx-v2 --input-tag pre-v2` inserts and waits
- No change to existing `--from`, `--max-retries` etc. (they are handled by River now, not the journal-based pipeline)

---

### Phase 1.6: Journal Removal (Cleanup)

**Goal:** Remove the old journal-based pipeline runner since River now handles durability.

- `internal/pipeline/pipeline.go` — keep for reference or archive
- `internal/journal/` — remove
- `.journal/`, `.journal-index/` — remove from `.gitignore` cleanup

The `internal/pipeline.Pipeline` and `internal/journal.Journal` types are no longer used by any CLI. The `internal/stage/*Stage` functions are still used by the River workers.

**Files to remove:**
- `internal/pipeline/pipeline.go`
- `internal/journal/journal.go`
- `internal/journal/journal_test.go`

**Deliverables:**
- All journal references removed
- `go build ./...` passes

---

## File Structure After Phase 1

```
cmd/
  preprocess/main.go          — Thin River job trigger
  index/main.go               — Thin River job trigger
  workerd/main.go             — River worker daemon (optional, or combined with CLI)

internal/
  db/
    db.go                     — PG connection pool
    migrate.go                — Schema migration runner
    migrations/
      001_create_workflow_tables.sql

  types/
    document.go               — ✓ Existing
    pipeline.go               — ✓ Existing (may be removed in 1.6)
    indexing.go               — ✓ Existing
    workflow.go               — NEW: Workflow, WorkflowStep structs

  workflow/
    store.go                  — NEW: Workflow CRUD + LoadState
    poll.go                   — NEW: PollUntilDone helper
    preprocess_worker.go      — NEW: CloneWorker, PreprocessWorker, VerifyWorker
    index_worker.go           — NEW: ParseWorker, ChunkWorker, EmbedWorker, StoreWorker

  config/config.go            — Extended: Tag, InputTag, DatabaseURL
  stage/                      — ✓ Existing (called by River workers)
  pipeline/pipeline.go        — ⚠ May be removed (replaced by River)
  journal/                    — ⚠ May be removed (replaced by River + Postgres)
```

---

## Testing Strategy

High-quality tests are a first-class deliverable for every sub-phase. Every new file must have a corresponding `_test.go` file with tests that are deterministic, fast, and exercise realistic failure modes.

### Principles

1. **Deterministic** — no flaky tests. Tests that hit external services (PG, Qdrant, LLM) must handle connection failures gracefully (skip, retry once, or use test doubles).
2. **Fast** — store tests target a real PG (docker-compose or testcontainers), not mocks. Worker tests skip River entirely (call the exported step logic directly). Integration tests are kept minimal — one happy path + one failure mode per workflow.
3. **Failure-first** — for each function, test the error path before the success path. River's value is durability under failure, so test what happens when PG is down, when a step returns an error, when `LoadState` finds no prior steps, etc.
4. **Table-driven** — use subtests (`t.Run`) for multiple inputs/expectations. Avoid copy-paste test functions.

### Per-Layer Standards

#### Store Tests (`internal/workflow/store_test.go`)

Use a real Postgres instance. Two options:
- **Recommended (simpler):** run `docker compose up -d postgres` before tests. The test connects via `DATABASE_URL` env var (fallback to default localhost). Skip if PG is unreachable.
- **Alternative:** use `testcontainers-go` for an isolated PG per test run (slower but fully isolated).

Test pattern:

```go
func TestStore_CreateAndGetWorkflow(t *testing.T) {
    pool := connectOrSkip(t)
    defer pool.Close()
    runMigrations(t, pool)

    store := workflow.NewStore(pool)
    id, err := store.CreateWorkflow(ctx, "preprocess", "pre-test-1", map[string]any{
        "repo_url": "https://example.com/repo.git",
    })
    require.NoError(t, err)
    require.NotEmpty(t, id)

    wf, err := store.GetWorkflow(ctx, id)
    require.NoError(t, err)
    assert.Equal(t, "preprocess", wf.Type)
    assert.Equal(t, "pre-test-1", wf.Tag)
    assert.Equal(t, "pending", wf.Status)
    assert.Equal(t, "https://example.com/repo.git", wf.InputParams["repo_url"])
}

func connectOrSkip(t *testing.T) *pgxpool.Pool {
    t.Helper()
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        dsn = "postgres://rag:rag@localhost:5432/rag?sslmode=disable"
    }
    pool, err := pgxpool.New(context.Background(), dsn)
    if err != nil {
        t.Skipf("postgres not available: %v", err)
    }
    return pool
}
```

**Required test cases for Store:**

| Method | Tests |
|--------|-------|
| `CreateWorkflow` | creates with valid params, rejects empty tag |
| `GetWorkflow` | returns existing, returns error for non-existent ID |
| `UpdateWorkflowStatus` | valid transitions (pending→running→succeeded), invalid transitions (succeeded→running) |
| `CreateStep` | creates with valid workflow ID, fails for non-existent workflow (FK constraint) |
| `UpdateStepStatus` | sets error on failure, stores output JSON on success, validates FK |
| `GetSteps` | returns steps in order, empty slice for no steps |
| `ListWorkflows` | filters by type, tag, status; paginates with limit/offset |
| `LoadState` | merges input_params + all completed step outputs; empty when no completed steps |

#### Worker Tests (`internal/workflow/preprocess_worker_test.go`, `index_worker_test.go`)

Test the step logic **directly** — do NOT enqueue real River jobs. Isolate by calling the underlying stage function with a config derived from job args.

Pattern — extract a helper that runs a worker's step logic without River:

```go
// In preprocess_worker.go, export a testable helper:
func RunCloneStep(ctx context.Context, args CloneArgs) (*types.StageResult, error) {
    cfg := &config.Config{
        RepoURL:  args.RepoURL,
        RepoPath: args.RepoPath,
    }
    stage := stagepkg.CloneStage(cfg)
    return stage.Run(ctx, make(map[string]any))
}

// In preprocess_worker_test.go:
func TestRunCloneStep_Success(t *testing.T) {
    repo := t.TempDir()
    args := CloneArgs{
        WorkflowID: "test-wf",
        Tag:        "pre-test",
        RepoURL:    "https://gitlab.com/gitlab-com/content-sites/handbook.git",
        RepoPath:   repo,
    }

    // This actually clones the repo — use a small test repo or mock git
    // For unit tests, test with a local git repo we create
    initBareGitRepo(t, repo)

    result, err := RunCloneStep(context.Background(), args)
    require.NoError(t, err)
    assert.Equal(t, repo, result.Output["repo_path"])
}
```

**Required test cases per worker:**

| Worker | Success tests | Failure tests |
|--------|---------------|---------------|
| `CloneWorker` | clones to correct path, reuses existing repo | invalid URL, no disk space, git binary missing |
| `PreprocessWorker` | processes files to correct output dir, respects `IncludeDirs` | input dir missing, permission denied |
| `VerifyWorker` | writes report, passes on clean output | output dir missing, verification fails |
| `ParseWorker` | walks correct `input_tag` path, returns documents | path missing, empty dir |
| `ChunkWorker` | creates correct number of chunks, respects size/overlap | empty document list |
| `EmbedWorker` | pairs chunks with embeddings | API unreachable, rate limit, auth failure |
| `StoreWorker` | creates named collection, upserts points | Qdrant unreachable, wrong vector size |

For dependencies that are expensive or non-deterministic (git, LLM API, Qdrant), use either:
- **Interface-based mocking** — wrap the external call behind an interface, inject a test double
- **Staged test fixtures** — pre-create the expected state on disk or in Qdrant before the test

#### Integration Tests (`cmd/workerd/main_test.go` or `cmd/preprocess/main_test.go`)

One happy-path integration test per workflow type, run only when `TEST_INTEGRATION=1` is set:

```go
func TestIntegration_PreprocessWorkflow(t *testing.T) {
    if os.Getenv("TEST_INTEGRATION") == "" {
        t.Skip("set TEST_INTEGRATION=1 to run")
    }

    pool := connectToPG(t)
    runMigrations(t, pool)
    store := workflow.NewStore(pool)

    // Create workflow + enqueue job via real River client
    riverClient := startRiverWorker(t, pool, store)
    wfID := createWorkflow(t, store, "preprocess", "integ-test", map[string]any{
        "repo_url": "https://gitlab.com/gitlab-com/content-sites/handbook.git",
    })
    insertJob(t, riverClient, &CloneArgs{WorkflowID: wfID, Tag: "integ-test", RepoURL: "...", RepoPath: t.TempDir()})

    // Poll until done (with timeout)
    err := workflow.PollUntilDone(ctx, store, wfID, 1*time.Second)
    require.NoError(t, err)

    // Verify artifacts exist
    assert.DirExists(t, filepath.Join("artifacts", "preprocessing", "integ-test", "output"))
}
```

#### CLI Tests (`cmd/preprocess/main_test.go`, `cmd/index/main_test.go`)

Test the CLI wrapper in isolation by injecting a fake store and River client. Do NOT trigger real jobs.

```go
func TestPreprocessCLI_InsertsRiverJob(t *testing.T) {
    // Override db.Connect and river.NewClient with test doubles
    // Verify that:
    //   1. CreateWorkflow was called with the right params
    //   2. Insert was called with CloneArgs containing the right RepoURL
    //   3. PollUntilDone was called with the returned workflow ID
}
```

### Test Fixtures

Pre-created test data lives in `testdata/` at the package level:

```
internal/workflow/testdata/
  workflows.sql           — INSERT statements for store tests
  empty_output/           — empty directory for parse tests
  preprocessed_sample/    — a few cleaned .md files for chunk/embed tests
```

### Running Tests

```powershell
# All unit tests (skip integration)
go test ./internal/workflow/...

# With integration tests (requires docker-compose services)
$env:TEST_INTEGRATION=1
go test ./cmd/... -count=1 -timeout=300s
```

### CI Expectations

| Check | Required | What it validates |
|-------|----------|-------------------|
| `go test ./internal/...` | ✅ | All unit tests pass |
| `go test ./cmd/...` | ✅ | All unit tests pass (integration skipped) |
| `go vet ./...` | ✅ | No suspicious constructs |
| `go build ./...` | ✅ | Compiles clean |
| **Full integration** | ⏳ Optional | `TEST_INTEGRATION=1` with docker-compose services up |

---

## Key Design Decisions

### 1. Worker runs existing stage logic (no rewrite)

River workers do **not** reimplement clone/preprocess/embed/store. They call the existing `stagepkg.CloneStage(cfg)` etc. with a temporary `Config` reconstructed from job args. This minimizes risk and keeps the existing test coverage relevant.

### 2. State flows through Postgres, not River

River job args carry `WorkflowID` and pipeline params. The actual workflow state (step outputs) lives in the `workflow_steps` table. Each worker loads the accumulated state via `store.LoadState()` before executing its step. This keeps River's job payload small and makes workflow state independently queryable by the API.

### 3. Tag determines all paths

All filesystem paths are derived from the workflow type and tag:

| Content | Path |
|---------|------|
| Cloned repo | `artifacts/preprocessing/{tag}/repo/` |
| Preprocessed output | `artifacts/preprocessing/{tag}/output/` |
| Indexing input | `artifacts/preprocessing/{input_tag}/output/` |
| Qdrant collection | `{tag}` |

This gives tag-based isolation without any config file or state coordination. The artifact tree is self-documenting:

```
artifacts/
  preprocessing/
    pre-v2/
      repo/            # cloned handbook repo
      output/          # cleaned markdown
    pre-v3/
      repo/
      output/
  indexing/
    idx-v2-fixed-512/
      metadata.json    # indexing params, doc count, etc.
    idx-v2-recursive-256/
      metadata.json
```

Note: indexing produces a Qdrant collection (not files), so `artifacts/indexing/{tag}/` stores metadata only.

### 4. Separate worker daemon (optional)

Workers can run in the same process as the CLI (for simplicity) or as a standalone `cmd/workerd` daemon (for production). The daemon pattern is recommended for Docker Compose:

```yaml
services:
  workerd:
    build: .
    command: ["workerd"]
    depends_on: [postgres, qdrant]
    environment:
      DATABASE_URL: postgres://rag:rag@postgres:5432/rag
```

### 5. River retries replace journal-based retries

The old `pipeline.Pipeline` has its own retry loop with exponential backoff and journal caching. With River, retries are handled by River's native retry mechanism (`MaxAttempts`, `BackoffSchedule`). The journal can be removed entirely.

---

## Migration Path

| Step | Action | Risk |
|------|--------|------|
| 1 | Add Postgres + River, deploy alongside existing CLIs | None (CLIs unchanged) |
| 2 | Rewrite CLIs to insert River jobs | Old journal-based runs abandoned; new runs use River |
| 3 | Keep old CLI as `--legacy` flag for rollback | Low |
| 4 | Remove journal code | Old `.journal` files orphaned; delete manually |

The migration is safe because:
- The existing CLIs use `./handbook/` and `./output/` — entirely separate from the new `artifacts/` tree
- Old preprocessed output in `./output/` coexists with new `artifacts/preprocessing/{tag}/output/`
- Old Qdrant collection `"document_chunks"` coexists with new `"{tag}"` collections

---

## Appendix: Go Pseudo-Code for Key Files

### `cmd/workerd/main.go` — Worker daemon

```go
func main() {
    pool, _ := db.Connect(context.Background())
    defer pool.Close()
    db.Migrate(pool)

    store := workflow.NewStore(pool)

    workers := river.NewWorkers()
    river.AddWorker(workers, &CloneWorker{store, riverClient})
    river.AddWorker(workers, &PreprocessWorker{store, riverClient})
    river.AddWorker(workers, &VerifyWorker{store, riverClient})
    river.AddWorker(workers, &ParseWorker{store, riverClient})
    river.AddWorker(workers, &ChunkWorker{store, riverClient})
    river.AddWorker(workers, &EmbedWorker{store, riverClient})
    river.AddWorker(workers, &StoreWorker{store, riverClient})

    riverClient, _ := river.NewClient(pool, &river.Config{
        Queues:  map[string]*river.QueueConfig{"default": {MaxWorkers: 5}},
        Workers: workers,
    })

    // Block forever
    select {}
}
```

### `cmd/preprocess/main.go` — Thin CLI

```go
func run(args []string) error {
    cfg := config.Load()
    cfg.Tag = resolveTag(cfg.Tag) // "pre-20260603-143022"

    // Derive paths from tag — no need for cfg.RepoPath or cfg.OutputPath
    repoPath := filepath.Join("artifacts", "preprocessing", cfg.Tag, "repo")
    outPath  := filepath.Join("artifacts", "preprocessing", cfg.Tag, "output")

    pool, _ := db.Connect(context.Background())
    defer pool.Close()

    store := workflow.NewStore(pool)
    wfID, _ := store.CreateWorkflow(ctx, "preprocess", cfg.Tag, map[string]any{
        "repo_url":     cfg.RepoURL,
        "include_dirs": cfg.IncludeDirs,
    })

    riverClient, _ := river.NewClient(pool, &river.Config{...})
    riverClient.Insert(ctx, &CloneArgs{
        WorkflowID: wfID,
        Tag:        cfg.Tag,
        RepoURL:    cfg.RepoURL,
        RepoPath:   repoPath,
    })

    return workflow.PollUntilDone(ctx, store, wfID, 2*time.Second)
}
```
