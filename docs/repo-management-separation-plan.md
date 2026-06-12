# Repo Management Separation Plan

Separates repository operations (clone, pull, delete) from preprocessing into
independent, first-class concerns.

## Motivation

The current `PreprocessWorker` combines git operations and markdown preprocessing
in a single River job. This couples two fundamentally different concerns:

1. **Repository lifecycle** — clone, pull, update, delete, list
2. **Content preprocessing** — Hugo → clean markdown transforms

Splitting them enables:

- **Independent repo updates** — pull latest without triggering re-preprocessing
- **Multi-config preprocessing** — clone once, preprocess N times with different `IncludeDirs`/`BaseURL`
- **Repo management UI/CLI** — dedicated API surface for repo CRUD
- **Cleaner architecture** — each concern is smaller, testable, and evolvable in isolation

## Tradeoff Analysis

| Concern | Current Design | After Split |
|---------|---------------|-------------|
| API calls for full pipeline | 1 | 2 (clone then preprocess) |
| Clone per config variant | 1 per tag | 1 total, N preprocess runs |
| Repo update without preprocess | Not possible | `POST /repos/{name}/pull` |
| Concurrency model | Single job owns repo | Shared repo, accept race |
| Code complexity | ~550 lines in one file | Two packages, cleaner separation |
| Failure modes | Simple (one job) | Coordination between repo + preprocess |

### Behavioral Decisions

- **Concurrent update during preprocessing**: Accept the race. Preprocessing reads
  whatever files are present. No locking.
- **Delete repo while preprocessing running**: Job fails with clear error. No
  dangling-reference protection.
- **Missing repo on preprocess trigger**: Worker fails immediately with descriptive
  error, not a raw filesystem error.

## New Artifact Layout

```
artifacts/
  repos/
    handbook/
      .repo-meta.json          # {url, branch, cloned_at}
      repo/                     # shallow git clone
        content/
          handbook/
            ...
  preprocessing/
    v15/
      output/                   # preprocessor output (unchanged)
        handbook/
          ...
        _verification_report.json
```

Repos are shared across preprocessing runs. Each preprocessing tag reads from
`artifacts/repos/<repo_name>/repo/content/` and writes to its own output directory.

## New API Surface

### Repo Management (synchronous)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/repos` | Clone a repository |
| `GET` | `/api/v1/repos` | List all cloned repos |
| `GET` | `/api/v1/repos/{name}` | Get repo info |
| `POST` | `/api/v1/repos/{name}/pull` | Pull latest changes |
| `DELETE` | `/api/v1/repos/{name}` | Delete a repo |

### Preprocessing (River worker, unchanged pattern)

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/workflows/preprocess` | Trigger preprocessing |

Request body changes from `repo_url` to `repo_name`.

### Example Workflow

```bash
# 1. Clone repo
curl -X POST /api/v1/repos \
  -d '{"name": "handbook", "url": "https://...", "branch": "main"}'

# 2. Preprocess with config A
curl -X POST /api/v1/workflows/preprocess \
  -d '{"repo_name": "handbook", "tag": "v15", "base_url": "https://handbook.gitlab.com"}'

# 3. Preprocess with config B (same repo, different output)
curl -X POST /api/v1/workflows/preprocess \
  -d '{"repo_name": "handbook", "tag": "v15-footer", "include_dirs": ["content/handbook"]'

# 4. Update repo
curl -X POST /api/v1/repos/handbook/pull

# 5. Re-preprocess with updated content
curl -X POST /api/v1/workflows/preprocess \
  -d '{"repo_name": "handbook", "tag": "v15.1", "base_url": "https://handbook.gitlab.com"}'
```

## Implementation Plan

### Phase 1: RepoManager Package

**New file: `internal/repomanager/repomanager.go`**

```go
package repomanager

type RepoManager struct {
    artifactsDir string
}

type RepoInfo struct {
    Name      string    `json:"name"`
    URL       string    `json:"url"`
    Branch    string    `json:"branch"`
    ClonedAt  time.Time `json:"cloned_at"`
}

type repoMeta struct {
    URL       string    `json:"url"`
    Branch    string    `json:"branch"`
    ClonedAt  time.Time `json:"cloned_at"`
}
```

**Methods:**

| Method | Signature | Behavior |
|--------|-----------|----------|
| `New` | `New(artifactsDir string) *RepoManager` | Constructor |
| `Clone` | `Clone(ctx, name, url, branch string) error` | Shallow clone to `repos/<name>/repo/`, write `.repo-meta.json`. If dir exists, return error. |
| `Pull` | `Pull(ctx, name string) error` | `fetch --all && checkout <branch> && pull --ff-only` in existing repo. Branch read from meta. |
| `Delete` | `Delete(name string) error` | Remove `repos/<name>/` directory. |
| `Get` | `Get(name string) (*RepoInfo, error)` | Read `.repo-meta.json`, return info. |
| `List` | `List() ([]RepoInfo, error)` | Scan `repos/` for dirs containing `.repo-meta.json`. |
| `RepoPath` | `RepoPath(name string) string` | Return `repos/<name>/repo/` path. |
| `Exists` | `Exists(name string) bool` | Check if repo dir + meta file exist. |

**Git helpers** — moved from `preprocess_worker.go`:

- `gitClone(ctx, url, targetPath string) error` — adds `branch` parameter (currently hardcoded to `main`)
- `gitUpdate(ctx, repoPath, branch string) error` — uses branch from meta
- `runGitTransient(ctx, repoPath, desc string, args ...string) error`
- `isTransientGitError(err error, stderr string) bool`
- `newGitBackoff() *backoff.ExponentialBackOff`

**Clone behavior:**
```
git -c core.longpaths=true clone --depth 1 --single-branch --branch <branch> <url> <target>
```

**Pull behavior:**
```
git fetch --all
git checkout <branch>
git pull --ff-only
```

Falls back to `git checkout -b <branch> origin/<branch>` if branch doesn't exist locally.

**Meta file** (`artifacts/repos/<name>/.repo-meta.json`):
```json
{
  "url": "https://gitlab.com/gitlab-com/gitlab-handbook.git",
  "branch": "main",
  "cloned_at": "2026-06-12T10:00:00Z"
}
```

**New file: `internal/repomanager/repomanager_test.go`**

Test cases:
- `TestClone_Success` — clone a repo, verify meta file and directory exist
- `TestClone_AlreadyExists` — returns error when repo dir exists
- `TestPull_Success` — pull updates existing repo
- `TestPull_NotFound` — returns error when repo doesn't exist
- `TestDelete_Success` — removes repo directory
- `TestDelete_NotFound` — returns error (or no-op, TBD)
- `TestGet_Success` — reads meta file
- `TestGet_NotFound` — returns error
- `TestList_Empty` — returns empty slice
- `TestList_Multiple` — returns all repos
- `TestList_SkipsInvalidDirs` — ignores dirs without meta file

All tests use `t.TempDir()` for `artifactsDir`. Mock git commands using test helper
that creates a fake git repo (bare repo or just the directory structure).

### Phase 2: Repo API Endpoints

**New file: `internal/api/router_repo.go`**

```go
type RepoRouter struct {
    mgr *repomanager.RepoManager
}

func NewRepoRouter(mgr *repomanager.RepoManager) *RepoRouter
func (r *RepoRouter) Register(mux chi.Router)
```

**Handlers:**

| Handler | Method | Path | Request Body | Response |
|---------|--------|------|--------------|----------|
| `createHandler` | POST | `/` | `CreateRepoRequest` | 201 + `RepoResponse` |
| `listHandler` | GET | `/` | — | 200 + `[]RepoResponse` |
| `getHandler` | GET | `/{name}` | — | 200 + `RepoResponse` |
| `pullHandler` | POST | `/{name}/pull` | — | 200 + `RepoResponse` |
| `deleteHandler` | DELETE | `/{name}` | — | 204 |

**Route registration:**
```go
func (r *RepoRouter) Register(mux chi.Router) {
    mux.Post("/", r.createHandler)
    mux.Get("/", r.listHandler)
    mux.Get("/{name}", r.getHandler)
    mux.Post("/{name}/pull", r.pullHandler)
    mux.Delete("/{name}", r.deleteHandler)
}
```

**New types in `internal/api/types.go`:**

```go
type CreateRepoRequest struct {
    Name   string `json:"name"`
    URL    string `json:"url"`
    Branch string `json:"branch,omitempty"` // defaults to "main"
}

func (r CreateRepoRequest) Validate() error {
    if r.Name == "" {
        return &validationError{"name is required"}
    }
    if r.URL == "" {
        return &validationError{"url is required"}
    }
    return nil
}

type RepoResponse struct {
    Name     string `json:"name"`
    URL      string `json:"url"`
    Branch   string `json:"branch"`
    ClonedAt string `json:"cloned_at"`
}
```

### Phase 3: Register in Server

**Modify: `internal/api/server.go`**

Add to `NewWithDeps()` after existing router registrations:

```go
repoMgr := repomanager.New(cfg.ArtifactsDir)
r.Route("/api/v1/repos", func(r chi.Router) {
    NewRepoRouter(repoMgr).Register(r)
})
```

### Phase 4: Simplify PreprocessWorker

**Modify: `internal/workflow/preprocess_worker.go`**

**Change `PreprocessArgs`:**
```go
type PreprocessArgs struct {
    Tag         string   `json:"tag"`
    RepoName    string   `json:"repo_name"`    // was RepoURL
    BaseURL     string   `json:"base_url"`
    IncludeDirs []string `json:"include_dirs,omitempty"`
}
```

**Remove from this file:**
- `cloneRepo()` function
- `gitCloneWithRetry()` function
- `gitClone()` function
- `gitUpdateWithRetry()` function
- `gitUpdate()` function
- `runGitTransient()` function
- `isTransientGitError()` function
- `newGitBackoff()` function
- `repoNotFoundRE` variable

**Keep in this file:**
- `PreprocessWorker.Work()` — simplified
- All `verify*` functions
- All `check*` functions
- All helper functions for markdown counting, size computation, etc.
- `readCheckpoint()` and `saveCheckpoint()` — simplified (no `clone_done`)

**Simplified `Work()` method:**

```go
func (w *PreprocessWorker) Work(ctx context.Context, job *river.Job[PreprocessArgs]) error {
    logger := slog.With("job_id", job.ID, "worker", "preprocess")
    logger.Debug("starting preprocess workflow")

    args := job.Args
    repoPath := path.Join("artifacts", "repos", args.RepoName, "repo")
    outputPath := path.Join("artifacts", "preprocessing", args.Tag, "output")

    // Validate repo exists
    if _, err := os.Stat(repoPath); os.IsNotExist(err) {
        return fmt.Errorf("repo %q not found — clone it first via POST /api/v1/repos", args.RepoName)
    }

    checkpoint := readCheckpoint(job)

    if !checkpoint["preprocess_done"] {
        logger.Debug("running preprocess step")
        srcDir := filepath.Join(repoPath, "content")
        _, err := preprocessor.ProcessAllFiles(ctx, srcDir, args.IncludeDirs, outputPath, 10, args.BaseURL)
        if err != nil {
            return fmt.Errorf("preprocess: %w", err)
        }
        if err := saveCheckpoint(ctx, w.Client, job, "preprocess_done", checkpoint); err != nil {
            return fmt.Errorf("save checkpoint after preprocess: %w", err)
        }
        checkpoint["preprocess_done"] = true
        logger.Debug("preprocess step completed")
    }

    logger.Debug("running verify step")
    srcDir := filepath.Join(repoPath, "content")
    if err := verifyOutput(srcDir, outputPath, args.IncludeDirs); err != nil {
        return fmt.Errorf("verify: %w", err)
    }

    logger.Info("preprocess workflow complete", "tag", args.Tag)
    return nil
}
```

**Checkpoint simplification:**

`readCheckpoint` and `saveCheckpoint` stay but now only track `preprocess_done`.
No more `clone_done` checkpoint. The functions themselves don't change structurally.

### Phase 5: Update API Types and Service

**Modify: `internal/api/types.go`**

Change `PreprocessRequest`:
```go
type PreprocessRequest struct {
    RepoName    string   `json:"repo_name"`   // was RepoURL
    Tag         string   `json:"tag"`
    BaseURL     string   `json:"base_url"`
    IncludeDirs []string `json:"include_dirs,omitempty"`
}

func (r PreprocessRequest) Validate() error {
    if r.RepoName == "" {
        return &validationError{"repo_name is required"}
    }
    if r.Tag == "" {
        return &validationError{"tag is required"}
    }
    return nil
}
```

**Modify: `internal/api/service_workflow.go`**

Update `InsertPreprocess`:
```go
func (s *WorkflowService) InsertPreprocess(ctx context.Context, req PreprocessRequest) (*WorkflowResponse, error) {
    result, err := s.client.Insert(ctx, &workflow.PreprocessArgs{
        Tag:         req.Tag,
        RepoName:    req.RepoName,    // was RepoURL
        BaseURL:     req.BaseURL,
        IncludeDirs: req.IncludeDirs,
    }, nil)
    if err != nil {
        return nil, fmt.Errorf("insert preprocess job: %w", err)
    }
    return jobToResponse(result.Job, req.Tag), nil
}
```

### Phase 6: Update Tests

**Modify: `internal/workflow/preprocess_worker_test.go`**

- Update all `PreprocessArgs` in test fixtures: replace `RepoURL` with `RepoName`
- Create test repos using `t.TempDir()` + fake git repo structure (directory with `content/` subdir)
- Tests that previously mocked git operations now just set up a directory

**New tests for `internal/repomanager/repomanager_test.go`:**

Test all RepoManager methods using `t.TempDir()`. For git operations, either:
- Use `exec.Command("git", "init", ...)` to create real bare repos in temp dirs
- Or create the directory structure manually and skip actual git commands (unit tests)

### Phase 7: Update Documentation

**Modify: `AGENTS.md`**

Update the Package Map table:
```
| `internal/repomanager/` | Repo lifecycle — clone, pull, delete, list, meta I/O |
```

Update Pipeline Architecture section:
```
Repo Management (synchronous): Clone → Pull → Delete → List
     ↓
Preprocess (1 job): Validate repo exists → Preprocess → Verify → cleaned markdown on disk
     ↓
Index (1 job): Parse → Chunk → Embed → Store (Qdrant)
     ↓
Eval (1 job): Batch embed queries → parallel eval
```

Update Conventions:
```
- **Repo management is synchronous** — clone/pull/delete happen via REST API, not River jobs.
  Repos live at `artifacts/repos/<name>/` and are shared across preprocessing runs.
- **Preprocessing references repos by name** — `PreprocessArgs.RepoName` maps to
  `artifacts/repos/<name>/repo/`. The repo must exist before preprocessing is triggered.
```

## File Change Summary

| File | Action | Lines Changed (est.) |
|------|--------|---------------------|
| `internal/repomanager/repomanager.go` | **New** | ~200 |
| `internal/repomanager/repomanager_test.go` | **New** | ~250 |
| `internal/api/router_repo.go` | **New** | ~150 |
| `internal/api/types.go` | **Modify** | ~30 (add repo types, change PreprocessRequest) |
| `internal/api/server.go` | **Modify** | ~5 (register RepoRouter) |
| `internal/api/service_workflow.go` | **Modify** | ~5 (map RepoName) |
| `internal/workflow/preprocess_worker.go` | **Modify** | ~-150 (remove git helpers, simplify Work) |
| `internal/workflow/preprocess_worker_test.go` | **Modify** | ~20 (update fixtures) |
| `AGENTS.md` | **Modify** | ~15 (update package map + conventions) |

## Dependency Graph

```
repomanager (no external deps beyond stdlib + backoff)
    ↑
router_repo → api/types
    ↑
server.go registers both routers
    ↓
workflow/preprocess_worker → repomanager.RepoPath (path derivation only, no import)
```

Note: The preprocess worker does NOT import `repomanager`. It derives the repo path
using the same convention (`artifacts/repos/<name>/repo/`). This keeps the worker
package free of the repo management dependency.

## Migration Path

No database migration needed. The change is API-breaking for the preprocess endpoint
(`repo_url` → `repo_name`). Any existing clients or scripts calling the preprocess
endpoint must be updated.

Existing preprocessing artifacts at `artifacts/preprocessing/<tag>/repo/` are
**not migrated**. They can be deleted manually. New artifacts follow the split layout.
