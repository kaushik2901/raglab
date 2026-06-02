# Preprocessing Pipeline — Phase-Wise Agent Plan

This document breaks the preprocessing pipeline into small, independent work items that can be picked up by separate agents working in parallel. Each item specifies inputs, outputs, interface contracts, and acceptance criteria.

---

## Dependency Graph

```
Phase 1 (Skeleton + Tests)
  ├── 1.1 Go module + dirs
  ├── 1.2 Types package
  ├── 1.3 Config package
  └── 1.4 Unit tests (types, config)
       │
Phase 2 (Core infra + Tests — depends on Phase 1)
  ├── 2.1 Journal implementation
  ├── 2.2 Pipeline runner
  └── 2.3 Unit tests (journal, pipeline)
       │
Phase 3 (Components — all depend on Phase 1, no interdependency)
  ├── 3.1 Clone stage
  ├── 3.2 Includes resolver
  ├── 3.3 Shortcode stripper
  ├── 3.4 HTML processor
  ├── 3.5 Refs resolver
  └── 3.6 Verify stage
       │
Phase 4 (Orchestration — depends on Phase 3)
  ├── 4.1 Preprocessor orchestrator  (depends on 3.2–3.5)
  ├── 4.2 Preprocess stage           (depends on 4.1)
  └── 4.3 Verify stage wiring        (already 3.6)
       │
Phase 5 (CLI — depends on Phase 4 + 3.1 + 3.6)
  ├── 5.1 Wire cmd/preprocess/main.go
  └── 5.2 Makefile
       │
Phase 6 (Integration Testing)
  └── 6.1 Integration test on handbook data
```

---

## Phase 1: Project Skeleton

### 1.1 — Initialize Go Module & Directory Structure

**Input:** None
**Output:** `go.mod`, empty directories per the project tree

**Instructions:**
```powershell
mkdir cmd/preprocess, internal/{pipeline,stage,preprocessor,journal,config,types}
go mod init github.com/kaushik/rag-pipeline  # adjust module path
```

**Acceptance criteria:**
- `go.mod` exists with correct module path
- All directories under `cmd/`, `internal/` exist
- `go.sum` can be generated (`go mod tidy` succeeds)

---

### 1.2 — Types Package (`internal/types/`)

**Input:** None (design doc)
**Output:** Two files `document.go`, `pipeline.go`

**Interface contracts to define:**

```go
// document.go
package types

type Document struct {
    Path    string // relative path e.g. "content/docs/foo.md"
    Content string // full text content
    Size    int64
}

// pipeline.go
type StageID string

type StageRecord struct {
    Name       StageID
    Succeeded  bool
    Error      string
    StartedAt  time.Time
    FinishedAt time.Time
    InputHash  string // deterministic hash of stage inputs
}

type StageResult struct {
    Name   StageID
    Output map[string]any // each stage stores its own output keys
    Err    error
}
```

**Acceptance criteria:**
- Package compiles cleanly
- All types exported
- `Document` has `Path` and `Content` fields as shown

---

### 1.3 — Config Package (`internal/config/`)

**Input:** CLI flags / env vars
**Output:** Unified `Config` struct with load/save

**Fields:**
```go
type Config struct {
    RepoURL      string // e.g. "https://gitlab.com/gitlab-com/content-sites/handbook"
    RepoPath     string // local clone path
    OutputPath   string // cleaned markdown output
    MaxRetries   int    // default 3
    RetryBackoff time.Duration // default 5s
    LogLevel     string // debug/info/warn
}
```

**Functions:**
- `Load() (*Config, error)` — read from CLI flags and env vars
- `Validate() error`

**Acceptance criteria:**
- Parses CLI flags (use `flag` package)
- Falls back to env vars (e.g. `REPO_URL`, `OUTPUT_PATH`)
- Validation returns error for missing required fields

---

### 1.4 — Unit Tests for Phase 1 (`internal/types/types_test.go`, `internal/config/config_test.go`)

**Tests for `internal/types`:**
- `TestDocumentCreation` — Document with all fields set, zero-value Document
- `TestStageRecordCreation` — StageRecord with Name, Succeeded, Error, timestamps, InputHash, Output map
- `TestStageRecordNilOutput` — zero-value Output is nil
- `TestStageRecordFailedWithError` — Succeeded=false with error string
- `TestStageResultCreation` — with nil and non-nil Err
- `TestStageResultNilOutput` — zero-value Output is nil
- `TestStageIDType` — works as string alias

**Tests for `internal/config`:**
- `TestValidate_*` — each required field (empty RepoURL, RepoPath, OutputPath), negative MaxRetries, zero/negative RetryBackoff, valid config
- `TestEnvOrDefault` — env var set → returns it; not set → default; empty → default
- `TestIntEnvOrDefault` — valid int, invalid value, empty, fallback
- `TestDurationEnvOrDefault` — valid duration, invalid, empty, fallback
- `TestLoad_Defaults` — flag defaults via `flag.NewFlagSet`
- `TestLoad_WithFlags` — custom flag values parsed correctly

**Edge cases covered:**
- All three required fields reported independently
- Zero MaxRetries is valid (no retries) but negative is not
- Zero RetryBackoff rejected
- Invalid env var values fall back to default (no crash)
- Global `flag` state isolation via `NewFlagSet`

**Acceptance criteria:**
- 31 tests (9 types + 22 config) pass via `go test ./internal/types/ ./internal/config/`

---

## Phase 2: Pipeline Core Infrastructure

### 2.1 — Journal Implementation (`internal/journal/`)

**Input:** `internal/types` package
**Output:** `journal.go` (interface) + `gob.go` (file-backed implementation)

**Interface:**
```go
type Journal interface {
    Record(stage types.StageID, record types.StageRecord) error
    Load(stage types.StageID) (*types.StageRecord, error)
    HasSucceeded(stage types.StageID, inputHash string) (bool, error)
    Clear() error
}
```

**GobFileJournal:**
- Stores records as gob-encoded files in a `.journal/` directory
- One file per stage (e.g. `.journal/clone.gob`)
- On load: decode and return record

**Acceptance criteria:**
- Can record, load, and check success for a stage
- Clear removes all journal files
- Handles missing journal gracefully (returns nil, not error)
- Thread-safe (use `sync.Mutex`)

---

### 2.2 — Pipeline Runner (`internal/pipeline/`)

**Input:** `internal/types`, `internal/journal`
**Output:** `pipeline.go`

**Core types:**
```go
type Stage struct {
    Name     types.StageID
    Run      func(ctx context.Context, state map[string]any) (*types.StageResult, error)
    Requires []types.StageID // stages that must precede this one
}

type Pipeline struct {
    Stages  []Stage
    Journal journal.Journal
    Config  *config.Config
}
```

**Methods:**
- `func (p *Pipeline) Run(ctx context.Context) error`
  - For each stage:
    1. Compute input hash
    2. Check journal for prior success with same hash
    3. If cached, skip and restore state
    4. Else run stage with retry (exponential backoff)
    5. Save result to journal
    6. Merge output into shared state
- `func (p *Pipeline) RunFrom(ctx context.Context, from types.StageID) error` — resume support

**Progress callback:**
```go
type ProgressFunc func(name types.StageID, status string, progress float64)
```

**Acceptance criteria:**
- Runs stages in order
- Skips already-succeeded stages (idempotent)
- Retries on failure up to `Config.MaxRetries`
- On restart, loads journal and resumes from first failed stage
- Exits early if required dependency not met

---

### 2.3 — Unit Tests for Phase 2 (`internal/journal/gob_test.go`, `internal/pipeline/pipeline_test.go`)

**Tests for `internal/journal`:**
- `TestRecordAndLoad` — round-trip encode/decode preserves all fields (Name, Succeeded, Error, timestamps, InputHash, Output)
- `TestLoad_MissingStage` — returns nil, nil when no file exists
- `TestHasSucceeded_True/NoRecord/WrongHash/EmptyHashIgnoresHash/NotSucceeded` — all hashing and success states
- `TestClear_RemovesGobFiles` — removes only .gob files, leaves other files intact
- `TestClear_NoDir` — no error on non-existent directory
- `TestRecord_OverwritesExisting` — second Record replaces prior entry
- `TestGobEncoding_ComplexOutput` — map with string/int/bool/float64 values survives gob round-trip
- `TestGobEncoding_NilOutput` — nil Output is preserved
- `TestRecordAndLoadMultipleStages` — independent stage records don't interfere
- `TestConcurrentAccess` — 20 goroutines hitting Record/Load/HasSucceeded concurrently
- `TestConcurrentDifferentStages` — concurrent writes to different stage files

**Tests for `internal/pipeline`:**
- `TestRun_SingleStage` — one stage completes successfully
- `TestRun_MultipleStages` — ordered execution with state passing between stages
- `TestRun_DependencyMet` — stage requiring a preceding stage works
- `TestRun_DependencyNotMet` — stage requiring a missing stage fails with clear error
- `TestRun_SkipCachedStage` — second run doesn't re-execute cached stages
- `TestRun_RetryThenSuccess` — transient failure recovers on retry
- `TestRun_RetryExhausted` — persistent failure returns error after MaxRetries attempts
- `TestRun_ContextCancellation` — cancelled context propagates from stage to pipeline
- `TestRun_StageFailureSavesRecord` — failed stage still persisted to journal with Succeeded=false
- `TestRunFrom_Resume` — skips cached stages, runs remaining, restores state from journal
- `TestRunFrom_FirstStage` — runs from first stage when specified
- `TestRunFrom_UnknownStage` — returns error for non-existent stage
- `TestRun_EmptyStages` — no stages = no error
- `TestProgressCallbacks` — initialized/running/completed events dispatched appropriately
- `TestComputeInputHash_Deterministic` — same stage produces same hash
- `TestComputeInputHash_Different` — different stages produce different hashes
- `TestComputeInputHash_RequiresOrderIndependent` — dependency order doesn't affect hash
- `TestComputeInputHash_DifferentRequires` — different deps produce different hashes

**Bugs found and fixed during testing:**
- `rand.Int63n(0)` panic when RetryBackoff is very small — added guard for half ≤ 0
- `HasSucceeded` empty hash treated as exact match — changed to ignore hash when empty (needed for dependency validation)
- Global `flag` redefinition panic in tests — switched to `flag.NewFlagSet` per test

**Acceptance criteria:**
- 35 tests (16 journal + 19 pipeline) pass via `go test ./internal/journal/ ./internal/pipeline/`
- Concurrent access tests exercise mutex-based thread safety

---

## Phase 3: Components (Parallelizable)

### 3.1 — Clone Stage (`internal/stage/clone.go`)

**Depends on:** Phase 1 (types, config)
**Parallelizable with:** 3.2, 3.3, 3.4, 3.5, 3.6

**Input:** Config (RepoURL, RepoPath)
**Output:** Repo contents on disk

**Logic:**
```go
func CloneStage(cfg *config.Config) pipeline.Stage {
    return pipeline.Stage{
        Name: "clone",
        Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
            // If repoPath exists: git fetch + checkout default + pull --ff-only
            // Else: git clone url path
        },
    }
}
```

**Edge cases:**
- Network failure → return error (pipeline retry handles backoff)
- Detached HEAD → checkout default branch first
- Cleanup on partial clone failure

**Acceptance criteria:**
- Clones repo if not present
- Pulls latest if already present
- State output: `{"repo_path": "/path/to/repo"}`

---

### 3.2 — Includes Resolver (`internal/preprocessor/includes.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 3.1, 3.3, 3.4, 3.5, 3.6

**Input:** Raw markdown content string + repo root path
**Output:** Markdown with `{{% include "path" %}}` resolved

**Signature:**
```go
// ResolveIncludes replaces all {{% include "path" %}} directives
// with the content of the referenced file. Includes are resolved
// recursively. visited tracks the call stack to detect cycles.
func ResolveIncludes(content string, repoRoot string, visited map[string]bool) (string, error)
```

**Algorithm:**
1. Find `{{% include "...path..." %}}` using regex
2. Resolve path relative to repo root
3. Read file, check markdown extension
4. Recursively resolve includes in included content
5. Replace tag with resolved content
6. Detect cycles via `visited` set

**Edge cases:**
- Missing file → skip with warning, continue
- Non-markdown file → skip
- Circular include → return error with cycle path

**Acceptance criteria:**
- Simple include: `{{% include "snippets/foo.md" %}}` → inlines content
- Nested include: included file has its own include → resolved recursively
- Circular include detected and returns error
- Missing file skips with warning (doesn't halt)
- Handles paths with spaces, quoted paths

---

### 3.3 — Shortcode Stripper (`internal/preprocessor/shortcodes.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 3.1, 3.2, 3.4, 3.5, 3.6

**Input:** Raw markdown content string
**Output:** Markdown with shortcode tags stripped per strategy

**Signature:**
```go
type ShortcodeAction int
const (
    Remove       ShortcodeAction = iota // Remove entirely including inner content
    StripTags                           // Remove tags, keep inner content
    Resolve                             // Call a resolver function
)

// ShortcodeRule defines how to handle a specific shortcode
type ShortcodeRule struct {
    Name   string
    Action ShortcodeAction
}

func StripShortcodes(content string, rules []ShortcodeRule) string
```

**Default rules (from plan doc):**

| Shortcode                     | Action    |
| ----------------------------- | --------- |
| include                       | Resolve   |
| details, alert, panel         | StripTags |
| handbook-data-toc, youtube    | Remove    |
| member-by-name, member-by-gitlab | Remove |
| ref, relref                   | Resolve   |
| Unknown                       | StripTags |

**Handles:**
- Self-closing: `{{< foo bar="baz" >}}` → Remove
- Paired: `{{< foo >}}...{{< /foo >}}` → StripTags keeps inner content
- Markdown-mode: `{{% foo %}}...{{% /foo %}}` → same as `{{< >}}`
- Shortcodes with parameters: `{{< youtube id="abc123" >}}`
- Shortcodes spanning multiple lines

**Acceptance criteria:**
- Self-closing shortcode removed entirely
- Paired shortcode tags removed, inner content preserved
- Unknown shortcode: tags stripped, inner content preserved
- No effect on regular markdown text
- Handles nested shortcodes? (Document: nested is rare, treat inner as text)

---

### 3.4 — HTML Processor (`internal/preprocessor/html.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 3.1, 3.2, 3.3, 3.5, 3.6

**Input:** Raw markdown content string
**Output:** Markdown with HTML processed per rules

**Signature:**
```go
func ProcessHTML(content string) string
```

**Rules:**

| Element              | Action                                    |
| -------------------- | ----------------------------------------- |
| `<style>...</style>` | Remove entirely (including content)        |
| `<script>...</script>` | Remove entirely                         |
| `<iframe ...>`       | Remove entirely (or keep URL as reference) |
| `<img ...>`          | Keep alt text, remove tag                 |
| `<a href="x">t</a>`  | Keep `text [href]`                        |
| `<div>...</div>`     | Keep inner text, remove tags              |
| `<table>...</table>` | Flatten to text (space-separated cells)   |
| Other tags           | Keep inner text, remove tags              |

**Implementation approach:**
- Use regex-based approach (simpler than full HTML parser for these rules)
- Or use `golang.org/x/net/html` for robustness

**Acceptance criteria:**
- `<style>` and `<script>` blocks completely removed
- `<img>` replaced by alt text
- `<a>` converted to `text [url]`
- `<div>`, `<span>` stripped, inner text preserved
- `<table>` flattened to space-separated text content
- `<iframe>` removed entirely
- Handles multi-line HTML blocks
- Handles HTML attributes (class, id, style, etc.)

---

### 3.5 — Refs Resolver (`internal/preprocessor/refs.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 3.1, 3.2, 3.3, 3.4, 3.6

**Input:** Raw markdown content string + repo root path + current file path
**Output:** Markdown with `{{< ref "path" >}}` and `{{< relref "path" >}}` resolved

**Signature:**
```go
// ResolveRefs converts {{< ref "path" >}} to relative markdown links.
// It resolves the target path relative to the current file's location.
func ResolveRefs(content string, repoRoot string, currentFilePath string) (string, error)
```

**Logic:**
- `{{< ref "docs/foo" >}}` → `[docs/foo](docs/foo.md)`
- `{{< relref "foo" >}}` → `[foo](foo.md)` (relative path)
- Handle anchor: `{{< ref "docs/foo#section" >}}` → `[docs/foo#section](docs/foo.md#section)`
- If target file exists on disk, verify the path; if not, still generate link but log warning

**Acceptance criteria:**
- `{{< ref "path" >}}` converted to markdown link
- `{{< relref "path" >}}` converted to relative markdown link
- Anchors preserved
- Missing target files → still generates link, logs warning

---

### 3.6 — Verify Stage (`internal/stage/verify.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 3.1–3.5

**Input:** Source path (original repo `content/`), output path (cleaned markdown)
**Output:** Verification report (JSON)

**Checks:**
```go
type VerificationReport struct {
    Passed bool
    Checks []CheckResult
}

type CheckResult struct {
    Name   string
    Passed bool
    Detail string
}
```

**Checks to implement:**
1. **FileCountMatch** — same number of `.md` files in source and output
2. **DirectoryStructurePreserved** — same relative paths in output
3. **NoShortcodesRemain** — grep for `\{\{%?<` patterns in output
4. **NoRawHTMLRemains** — grep for `<[a-z]+[^>]*>` tags
5. **MinimumContentPerFile** — no empty output files
6. **TotalSizeSanity** — total bytes within reasonable range (e.g., 0.5x to 2x source)

**Signature:**
```go
func VerifyStage(cfg *config.Config) pipeline.Stage
```

**Acceptance criteria:**
- Returns report with pass/fail per check
- Writes report JSON to `{output}/_verification_report.json`
- Non-zero exit if any check fails

---

## Phase 4: Orchestration

### 4.1 — Preprocessor Orchestrator (`internal/preprocessor/preprocessor.go`)

**Depends on:** 3.2, 3.3, 3.4, 3.5

**Input:** Single `.md` file path, repo root
**Output:** Cleaned markdown content

**Logic (ordered processing):**
```go
func ProcessFile(filePath string, repoRoot string) (*types.Document, error) {
    content := readFile(filePath)

    // Step 1: Resolve includes (recursive)
    content, err = ResolveIncludes(content, repoRoot, make(map[string]bool))

    // Step 2: Strip shortcodes (except include, ref — already handled)
    rules := defaultShortcodeRules()
    content = StripShortcodes(content, rules)

    // Step 3: Process raw HTML
    content = ProcessHTML(content)

    // Step 4: Resolve refs/relref
    content, err = ResolveRefs(content, repoRoot, filePath)

    return &types.Document{
        Path:    relativePath(filePath, repoRoot),
        Content: content,
        Size:    int64(len(content)),
    }, nil
}
```

**Additional helper:**
```go
// ProcessAllFiles walks content/ dir, processes each .md file,
// writes results to output dir preserving directory structure.
func ProcessAllFiles(srcDir string, dstDir string, concurrency int) (int, error)
```

- Walk `srcDir` recursively
- Filter `.md` files
- Process each file (optionally concurrent, concurrency limit default 10)
- Write to `dstDir` preserving relative path
- Return count of processed files

**Acceptance criteria:**
- All four transforms applied in correct order
- Output written to destination with same directory structure
- Concurrent processing doesn't corrupt output
- Returns correct file count
- Non-`.md` files skipped (pass-through as-is?)

---

### 4.2 — Preprocess Stage (`internal/stage/preprocess.go`)

**Depends on:** 4.1

**Wires the preprocessor as a pipeline stage:**
```go
func PreprocessStage(cfg *config.Config) pipeline.Stage {
    return pipeline.Stage{
        Name: "preprocess",
        Requires: []types.StageID{"clone"},
        Run: func(ctx context.Context, state map[string]any) (*types.StageResult, error) {
            repoPath := state["repo_path"].(string)
            srcDir := filepath.Join(repoPath, "content")
            dstDir := cfg.OutputPath
            count, err := preprocessor.ProcessAllFiles(srcDir, dstDir, 10)
            return &types.StageResult{
                Output: map[string]any{"processed_count": count},
            }, err
        },
    }
}
```

**Acceptance criteria:**
- Reads `repo_path` from state (set by clone stage)
- Reads `content/` subdirectory from cloned repo
- Writes processed files to `cfg.OutputPath`
- Returns `processed_count` in output

---

## Phase 5: CLI Integration

### 5.1 — Wire `cmd/preprocess/main.go`

**Depends on:** Phase 4 (pipeline, clone, preprocess, verify stages)

**Main function:**
```go
func main() {
    cfg := config.Load()

    pipeline := pipeline.Pipeline{
        Journal: journal.NewGobFileJournal(".journal"),
        Config:  cfg,
        Stages: []pipeline.Stage{
            clone.CloneStage(cfg),
            preprocess.PreprocessStage(cfg),
            verify.VerifyStage(cfg),
        },
    }

    if err := pipeline.Run(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

**CLI flags:**
```
--repo-url     (default: https://gitlab.com/gitlab-com/content-sites/handbook)
--repo-path    (default: ./handbook)
--output       (default: ./output)
--max-retries  (default: 3)
--from         (resume from stage name)
```

**Acceptance criteria:**
- `go build ./cmd/preprocess` succeeds
- Running the binary with valid flags runs all stages
- `--from` flag resumes from a specific stage
- Exit 0 on success, non-zero on failure

---

### 5.2 — Makefile

**Depends on:** 5.1

```makefile
.PHONY: build clean run test

build:
    go build -o bin/preprocess ./cmd/preprocess

run: build
    ./bin/preprocess

clean:
    rm -rf bin/ output/ .journal/

test:
    go test ./...
```

**Acceptance criteria:**
- `make build` succeeds
- `make run` runs the pipeline

---

## Phase 6: Integration Testing

### 6.1 — Integration Test on Real Handbook

**Depends on:** Phase 5 (CLI)

**Script (`test/integration_test.go` or shell script):**
1. Clone handbook repo
2. Run preprocess pipeline
3. Run verify stage
4. Assert all verify checks pass
5. Check output structure matches source structure
6. Sample-check a few output files for remaining shortcodes/HTML

**Acceptance criteria:**
- Pipeline completes on real handbook data
- Verify stage passes all checks
- Output contains ~4,487 `.md` files

---

## Testing Philosophy

Unit tests are written **within each phase** alongside the implementation, not deferred to a separate phase. This ensures:

- **Immediate feedback:** Bugs are caught at implementation time, not weeks later
- **Test-first thinking:** Developers consider edge cases while writing code
- **Small, focused tests:** Each package has its own test file with targeted coverage
- **No regression risk:** Changes to a package are validated immediately

### Completed test counts (Phase 1 + 2):

| Package             | Tests | Key areas                               |
| ------------------- | ----- | --------------------------------------- |
| `internal/types`    | 9     | Document, StageRecord, StageResult, IDs |
| `internal/config`   | 22    | Validation, env helpers, flag parsing   |
| `internal/journal`  | 16    | Record/Load, hashing, clear, concurrency|
| `internal/pipeline` | 19    | Ordering, retry, caching, resume, state |
| **Total**           | **66**| All passing `go test ./internal/...`    |

### Remaining tests (to be written in later phases):

| Package               | Est. tests | When    |
| --------------------- | ---------- | ------- |
| `internal/preprocessor` (includes, shortcodes, html, refs, preprocessor) | ~40 | Phase 3 |
| `internal/stage` (clone, verify, preprocess) | ~15 | Phase 3 |
| Integration test      | 1          | Phase 6 |

---

## Agent Assignment Recommendations

| Agent | Work Item | Est. Effort | Dependencies |
| ----- | --------- | ----------- | ------------ |
| A     | 1.1 + 1.2 + 1.3 (Skeleton) | Small | None |
| B     | 2.1 (Journal) + tests | Small | Phase 1 |
| C     | 2.2 (Pipeline runner) + tests | Medium | Phase 1 |
| D     | 3.1 (Clone stage) | Small | Phase 1 |
| E     | 3.2 (Includes resolver) + tests | Small | Phase 1 |
| F     | 3.3 (Shortcode stripper) + tests | Small | Phase 1 |
| G     | 3.4 (HTML processor) + tests | Medium | Phase 1 |
| H     | 3.5 (Refs resolver) + tests | Small | Phase 1 |
| I     | 3.6 (Verify stage) + tests | Small | Phase 1 |
| J     | 4.1 (Preprocessor orchestrator) + tests | Medium | 3.2–3.5 |
| K     | 4.2 (Preprocess stage) | Small | 4.1 |
| L     | 5.1 (CLI main.go) | Small | Phase 4 + 3.1 + 3.6 |
| M     | 5.2 (Makefile) | Tiny | 5.1 |
| N     | 6.1 (Integration test) | Small | Phase 5 |

**Parallelization strategy:**
- **Wave 1:** A, B, C, D, E, F, G, H, I after A completes skeleton
- **Wave 2:** J, K after E, F, G, H (and their tests) complete
- **Wave 3:** L, M after J, K, D, I complete
- **Wave 4:** N after everything else

Total: ~14 agent tasks, completed in 4 waves with ~9 agents in parallel in wave 1.

Note: Each agent writes unit tests alongside implementation (not deferred).
