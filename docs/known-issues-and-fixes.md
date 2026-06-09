# Known Issues & Fixes

Catalog of all obvious issues found during codebase audit, ordered by implementation complexity.
✅ FIXED items have been resolved in code and are kept for historical reference.

---

## ✅ Fixed

### 1. One embedding failure abandons all remaining files ✅ FIXED

**Location**: `internal/workflow/index_worker.go:167-168`
**Resolution**: Embed and store errors are now gracefully skipped like parse/chunk errors. The `skipErrors` pattern (mutex-guarded slice) logs each failure and returns `nil` to the errgroup, so processing continues. Skipped files are reported at the end.

---

### 2. No streaming — entire file loaded in memory before processing ✅ FIXED

**Resolution**: The indexing pipeline now uses streaming throughout:
- `internal/parser/markdown.go` — `MarkdownParser` uses `bufio.Scanner` for line-by-line streaming, yields `Element` values one at a time.
- `internal/chunker/fixed.go` — `FixedChunker` reads `Element`s incrementally from the stream and emits `Chunk`s on a channel via a sliding-window word-level algorithm.
- `internal/workflow/index_worker.go:90-129` — `processFile` reads chunks from the channel in batches, embeds, and stores — never holding the full document or all chunks in memory.
- Old non-streaming `ParseFile`/`ParseDir` in parser.go have been removed (dead code).

---

### 3. Fixed 5-minute per-document timeout ✅ FIXED

**Location**: `internal/workflow/index_worker.go:133`
**Resolution**: Timeout is now configurable via `IndexArgs.DocTimeout` (River job param). The default was raised to 30 minutes. Callers (e.g. `cmd/index/main.go`) can set `--doc-timeout` to override.

---

### 4. Single Qdrant gRPC connection — no reconnection ✅ FIXED

**Location**: `internal/store/qdrant.go:48`
**Resolution**: `EnsureCollection`, `Store`, and `Search` all wrap their inner calls in a retry loop (3 attempts) that calls `reconnect()` on connection errors before retrying. `lastDSN` is stored on `Connect` for reconnection.

---

### 5. Reactive-only rate limiting (no proactive control) ✅ FIXED

**Location**: `internal/embedder/openai.go:84-104`, `internal/generator/generator.go:61-83`
**Resolution**: `RateLimitedEmbedder` and `RateLimitedGenerator` token-bucket wrappers are wired into the `New()` factories. Default rate: 100 RPM, configurable via `EMBEDDER_RATE_LIMIT_RPM` / `GENERATOR_RATE_LIMIT_RPM`.

---

## Quick Fixes — complete

### 6. Workerd caps at 5 concurrent jobs ✅ FIXED

**Resolution**: `MaxWorkers` is now `config.IntEnvOrDefault("WORKER_CONCURRENCY", 20)` in `cmd/workerd/main.go`.

### 7. Eval pipeline context leak ✅ FIXED

**Resolution**: `defer cancel()` replaced with explicit `cancel()` at the end of each loop iteration (and before early returns) in `internal/eval/pipeline.go`.

### 8. Workflow status transition "running" never set on workflow ✅ FIXED

**Resolution**: `runStep` in `internal/workflow/store.go` now calls `UpdateWorkflowStatus(ctx, wfID, "running")` (best-effort) after creating the step.

### 9. No `Content-Type` header setting in embedder/generator HTTP calls ✅ FIXED

**Resolution**: `option.WithHeader("Content-Type", "application/json")` added to both `newOpenAIEmbedder` and `NewOpenAI` client options.

### 10. `PollUntilDone` uses fixed 2-second interval ✅ FIXED

**Resolution**: `PollUntilDone` now uses exponential backoff: doubles each loop from the initial interval, capped at 30s.

### 11. Preprocessor `errgroup` uses `context.Background()` unconditionally ✅ FIXED

**Resolution**: `ProcessAllFiles` now accepts `ctx context.Context` as the first parameter and uses `errgroup.WithContext(ctx)`. Callers updated.

### 12. Memory store never garbage-collects conversations ✅ FIXED

**Resolution**: `RingBuffer` now tracks `lastAccessed` timestamps per conversation and exposes `Purge(threshold)` to delete idle conversations.

---

## Medium Changes (multi-file, moderate complexity)

### 13. No composite DB indexes

**Location**: `internal/db/migrations/001_initial.sql`
**Problem**: `ListWorkflows` queries with filters on `type`, `tag`, `status` but only single-column indexes exist. At 100k+ workflow rows, queries will be slow.

**Fix**: Add composite indexes:

```sql
CREATE INDEX IF NOT EXISTS idx_workflows_type_status ON workflows(type, status);
CREATE INDEX IF NOT EXISTS idx_workflows_tag_type ON workflows(tag, type);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_wf_status ON workflow_steps(workflow_id, status);
```

### 14. `eval_worker.go` creates a new Qdrant connection per eval run

**Location**: `internal/workflow/eval_worker.go:115-119`
**Problem**: Every eval workflow creates a new gRPC connection to Qdrant. If the workerd processes multiple eval jobs simultaneously, each has its own connection.
**At scale**: With concurrent eval runs, gRPC connection overhead multiplies.

**Fix**: Pass the Qdrant store as a shared dependency in the workerd setup (`cmd/workerd/main.go`), same pattern as `Store` and `EvalStore`.

### 15. Embedder returns `[]float64` — immediate conversion to `[]float32`

**Location**:
- `internal/embedder/openai.go:121` — `d.Embedding` is `[]float64`
- `internal/store/qdrant.go:175-178` — `toPoint` converts to `[]float32`
- `internal/retriever/retriever.go:50-53` — another conversion
- `internal/eval/pipeline.go:96-101` — `toFloat32` helper

**Problem**: Double memory: embeddings are stored as `[]float64` in `types.Embedding.Vector` and converted to `[]float32` on every store/search. At 10k chunks × 1536 dims = 60 MB extra per batch.
**At scale**: Wastes 60 MB per active batch, plus GC pressure from the conversion.

**Fix**: Change `types.Embedding.Vector` from `[]float64` to `[]float32`:

```go
type Embedding struct {
    ChunkID    string
    Vector     []float32  // was []float64
    Model      string
    Dimensions int
}
```

Then remove all `toFloat32` conversion sites.

### 16. No structured error types

**Location**: Throughout
**Problem**: All errors are string-wrapped with `fmt.Errorf`. No typed errors for retryable vs. non-retryable, connection errors, etc.

**Fix**: Define sentinel errors: `ErrRetryable`, `ErrConnection`, `ErrRateLimited`, etc.

---

## Complex Reworks (architectural, risk of regression)

### 17. Preprocessor walks directory twice

**Location**: `internal/preprocessor/preprocessor.go:68-98` (first walk to collect files) + `ProcessFile` (second walk via `ResolveIncludes` re-reading from disk)
**Problem**: The preprocessor first walks all source dirs to collect `.md` file paths, then processes each one. `ResolveIncludes` does additional `os.ReadFile` calls for included content. At 200k files, this doubles I/O.
**At scale**: 200k files × multiple reads = significant wall time.

**Actual fix**: The real double-walk is in verifying: `verify.go` re-walks both src and dst directories independently. Add a cache or pass the file list from preprocessing.

### 18. Missing `GOMEMLIMIT` — no Go memory limit set

**Location**: `cmd/workerd/main.go`, `cmd/index/main.go`, `cmd/preprocess/main.go`
**Problem**: Go's default memory behavior is to grow until the OS OOM killer steps in. No `GOMEMLIMIT` or `runtime/debug.SetMemoryLimit` is set.
**At scale**: OOM is unrecoverable.

**Fix**: Add to each `main.go`:

```go
import "runtime/debug"

func init() {
    debug.SetMemoryLimit(5 * 1024 * 1024 * 1024) // 5 GB default
}
```

Also respect `GOMEMLIMIT` env var (Go 1.19+).

### 19. No per-item error granularity in batch embedding

**Location**: `internal/embedder/openai.go:109-110`
**Problem**: If one chunk in a batch causes a 400 error, the entire batch fails. No way to identify which chunk caused the problem.
**At scale**: An occasional bad document could cause repeated failures.

**Fix**: If the API returns individual errors per item (some providers do), map them back to individual chunks. For OpenAI, which returns all-or-nothing, skip the batch and fall back to single-item embedding for the failed batch.

### 20. Shortcode parser re-scans content repeatedly

**Location**: `internal/preprocessor/shortcodes.go:22-93`
**Problem**: `StripShortcodes` uses `strings.Index()` to find close tags, re-scanning the same content. For a file with 100 shortcodes, worst-case O(n²).
**At scale**: Large files with many Hugo shortcodes will be slow.

**Fix**: Parse content left-to-right in a single pass, tracking open tag stack. Use a cursor position instead of repeated `strings.Index` calls.
