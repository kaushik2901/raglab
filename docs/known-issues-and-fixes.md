# Known Issues & Fixes

Catalog of all obvious issues found during codebase audit, ordered by impact severity.

---

## Critical — Will break at 10x+

### 1. One embedding failure abandons all remaining files

**Location**: `internal/workflow/index_worker.go:167-168`
**Problem**: When `emb.Embed()` or `qStore.Store()` fails for one file, the `errgroup` context is cancelled and `g.Wait()` returns the first error. All unprocessed files are abandoned and all already-processed work is wasted. Parse/chunk errors (lines 143-158) are gracefully skipped, but embed/store errors are not — inconsistent error handling.
**At scale**: With 200k files and any transient API error, the entire run is lost and must be restarted.

**Fix**: Wrap embed and store in graceful error handling like parse/chunk:

```
diff --git a/internal/workflow/index_worker.go b/internal/workflow/index_worker.go
--- a/internal/workflow/index_worker.go
+++ b/internal/workflow/index_worker.go
@@ -164,13 +164,17 @@ func RunIndexing(ctx context.Context, args IndexArgs) (*types.StageResult, erro
 			}

 			embeddings, err := emb.Embed(docCtx, chunks)
 			if err != nil {
-				return fmt.Errorf("embed %s: %w", relPath, err)
+				skipError(relPath, "embed", err)
+				return nil
 			}

+			// ... same for Store below
```

Create a helper `skipError` that adds to `skipErrors` under the mutex.

---

### 2. No streaming — entire file loaded in memory before processing

**Location**:

- `internal/parser/parser.go:62` — `io.ReadAll(f)`
- `internal/chunker/fixed.go:23` — `strings.Fields(doc.Content)` word-splits the whole doc
- `internal/workflow/index_worker.go:133-189` — entire lifecycle holds full doc + chunk embeddings

**Problem**: The pipeline is read-into-memory → process → store. For a 10 MB doc with 10k chunks at 1536-dim float32, each embedding batch = 10k × 1536 × 4 bytes ≈ 61 MB just for vectors. With 5 concurrent goroutines that's >300 MB active.
**At scale**: A single large doc or 200k small docs with 5 goroutines will OOM.

**Fix (short-term)**: Add a maximum file size check and skip large files with a warning. Can be removed when streaming is implemented.

```go
const maxFileSize = 50 * 1024 * 1024 // 50 MB

func ParseFile(filePath string, relPath string) (types.Document, error) {
    info, err := os.Stat(filePath)
    if err != nil {
        return types.Document{}, err
    }
    if info.Size() > maxFileSize {
        return types.Document{}, fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxFileSize)
    }
    // ... rest of function
}
```

**Fix (long-term)**: Stream chunks: read a chunk's worth of tokens → embed → store → repeat. Requires chunker refactor to accept `io.Reader`.

---

### 3. Fixed 5-minute per-document timeout

**Location**: `internal/workflow/index_worker.go:133`
**Problem**: `context.WithTimeout(ctx, 5*time.Minute)` covers the entire parse+chunk+embed+store cycle per file. Large documents or slow API responses will hit this.
**At scale**: At 100x, if rate limiting causes 30s delays per batch, a doc with 50 batches = 25 minutes, hitting the timeout.

**Fix**: Make the timeout proportional to doc size or configurable:

```go
timeout := 5 * time.Minute
if docSize := len(doc.Content); docSize > 0 {
    estimated := time.Duration(docSize/1000) * time.Second // 1s per KB heuristic
    if estimated > timeout {
        timeout = estimated
    }
}
docCtx, cancel := context.WithTimeout(ctx, timeout)
```

---

### 4. Single Qdrant gRPC connection — no reconnection

**Location**: `internal/store/qdrant.go:48`
**Problem**: `qdrant.NewGrpcClient` creates a single persistent gRPC connection. If Qdrant restarts or the connection drops, all `Store` and `Search` calls fail permanently.
**At scale**: At 100x indexing, the connection is under continuous load for hours. Any network blip aborts the entire run.

**Fix**: Wrap Qdrant calls in a retry loop that reconnects:

```go
func (s *QdrantStore) Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error {
    for attempt := 0; attempt < 3; attempt++ {
        err := s.storeOnce(ctx, collectionName, chunks)
        if err == nil {
            return nil
        }
        if isConnError(err) {
            s.client.Close()
            s.client = nil
            if reconnectErr := s.Connect(ctx, s.lastDSN); reconnectErr != nil {
                return fmt.Errorf("reconnect: %w", reconnectErr)
            }
            continue
        }
        return err
    }
    return fmt.Errorf("store failed after retries")
}
```

Also store `lastDSN` on `Connect` for reconnection.

---

### 5. Reactive-only rate limiting (no proactive control)

**Location**:

- `internal/embedder/openai.go:84-104` — 429 retry with `time.Sleep`
- `internal/generator/generator.go:61-83` — same pattern

**Problem**: Both embedder and generator block the calling goroutine with `time.Sleep` on 429. Multiple concurrent goroutines all backoff independently, causing thundering herd on retry. No token bucket, no request queueing.
**At scale**: At 100x, most wall time is spent sleeping.

**Fix**: Add a token bucket rate limiter that wraps the embedder/generator:

```go
// internal/embedder/ratelimit.go
type RateLimitedEmbedder struct {
    inner  Embedder
    bucket *rate.Limiter
}

func (r *RateLimitedEmbedder) Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error) {
    if err := r.bucket.Wait(ctx); err != nil {
        return nil, err
    }
    return r.inner.Embed(ctx, chunks)
}
```

And wrap at creation time in `internal/embedder/embedder.go:New()` and `internal/generator/generator.go:New()`. Default rate: 100 RPM for OpenAI, configurable via `EMBEDDER_RATE_LIMIT_RPM` / `GENERATOR_RATE_LIMIT_RPM`.

---

## High — Will degrade at 10x+, block at 100x

### 6. Workerd caps at 5 concurrent jobs

**Location**: `cmd/workerd/main.go:58`
**Problem**: `MaxWorkers: 5` is hardcoded. This caps pipeline throughput regardless of available CPU.
**At scale**: Indexing 200k files with 5 workers is extremely slow. The workerd is the bottleneck.

**Fix**: Make it configurable via env var with a reasonable default:

```go
maxWorkers := config.IntEnvOrDefault("WORKER_CONCURRENCY", 20)
Queues: map[string]river.QueueConfig{
    "default": {MaxWorkers: maxWorkers},
},
```

---

### 7. No composite DB indexes

**Location**: `internal/db/migrations/001_initial.sql`
**Problem**: `ListWorkflows` queries with filters on `type`, `tag`, `status` but only single-column indexes exist. At 100k+ workflow rows, queries will be slow.

**Fix**: Add composite indexes:

```sql
CREATE INDEX IF NOT EXISTS idx_workflows_type_status ON workflows(type, status);
CREATE INDEX IF NOT EXISTS idx_workflows_tag_type ON workflows(tag, type);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_wf_status ON workflow_steps(workflow_id, status);
```

---

### 8. Preprocessor walks directory twice

**Location**: `internal/preprocessor/preprocessor.go:68-98` (first walk to collect files) + `ProcessFile` (second walk via `ResolveIncludes` re-reading from disk)
**Problem**: The preprocessor first walks all source dirs to collect `.md` file paths, then processes each one. `ResolveIncludes` does additional `os.ReadFile` calls for included content. At 200k files, this doubles I/O.
**At scale**: 200k files × multiple reads = significant wall time.

**Fix**: Combine the collection and processing in a single walk, or add an in-memory cache for frequently included files.

```go
var (
    mdFiles []string
    mu      sync.Mutex
)

err = filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {
    if err != nil { return err }
    if info.IsDir() { return nil }
    if !isMarkdown(info.Name()) { return nil }

    mu.Lock()
    mdFiles = append(mdFiles, path)
    mu.Unlock()
    return nil
})
```

(This is already the pattern, but the second walk in `ProcessAllFiles` for collecting files before processing is what I mean — it's actually one walk.)

**Actual fix**: The real double-walk is in verifying: `verify.go` re-walks both src and dst directories independently. Add a cache or pass the file list from preprocessing.

---

### 9. Missing `GOMEMLIMIT` — no Go memory limit set

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

---

## Medium — Hardening / correctness

### 10. `eval_worker.go` creates a new Qdrant connection per eval run

**Location**: `internal/workflow/eval_worker.go:115-119`
**Problem**: Every eval workflow creates a new gRPC connection to Qdrant. If the workerd processes multiple eval jobs simultaneously, each has its own connection.
**At scale**: With concurrent eval runs, gRPC connection overhead multiplies.

**Fix**: Pass the Qdrant store as a shared dependency in the workerd setup (`cmd/workerd/main.go`), same pattern as `Store` and `EvalStore`.

---

### 11. Embedder returns `[]float64` — immediate conversion to `[]float32`

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

---

### 12. No per-item error granularity in batch embedding

**Location**: `internal/embedder/openai.go:109-110`
**Problem**: If one chunk in a batch causes a 400 error, the entire batch fails. No way to identify which chunk caused the problem.
**At scale**: An occasional bad document could cause repeated failures.

**Fix**: If the API returns individual errors per item (some providers do), map them back to individual chunks. For OpenAI, which returns all-or-nothing, skip the batch and fall back to single-item embedding for the failed batch.

---

### 13. Shortcode parser re-scans content repeatedly

**Location**: `internal/preprocessor/shortcodes.go:22-93`
**Problem**: `StripShortcodes` uses `strings.Index()` to find close tags, re-scanning the same content. For a file with 100 shortcodes, worst-case O(n²).
**At scale**: Large files with many Hugo shortcodes will be slow.

**Fix**: Parse content left-to-right in a single pass, tracking open tag stack. Use a cursor position instead of repeated `strings.Index` calls.

---

### 14. Eval pipeline context leak

**Location**: `internal/eval/pipeline.go:53-54`
**Problem**: `context.WithTimeout` is called inside the loop with `defer cancel()`. Deferred calls in a loop only fire when the function returns, so all per-question contexts pile up until the loop ends.
**At scale**: With 1000+ questions, 1000+ context objects accumulate.

**Fix**: Call `cancel()` explicitly at the end of each loop iteration instead of deferring.

```go
for i, q := range args.Questions {
    qCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
    // ... use qCtx
    cancel() // explicit, not defer
}
```

---

### 15. Workflow status transition "running" never set on workflow

**Location**: `internal/workflow/store.go:81-97`, `internal/workflow/preprocess_worker.go:54-62`
**Problem**: When a workflow is created, status is `"pending"`. The first worker calls `runStep` which creates a step with `"running"` status, but the **workflow itself** stays `"pending"` until `UpdateWorkflowStatus(ctx, wfID, "succeeded")` or a `runStep` error occurs. There's no `workflow → "running"` transition.
**At scale**: Console output shows all workflows as `"pending"` until they finish.

**Fix**: Set workflow to `"running"` when the first step starts:

```go
// In runStep, after CreateStep succeeds:
if err := s.UpdateWorkflowStatus(ctx, wfID, "running"); err != nil {
    // Best-effort — the step will still execute
    slog.Warn("failed to set workflow running", "err", err)
}
```

---

## Low — Polish / observability

### 16. No `Content-Type` header setting in embedder/generator HTTP calls

**Problem**: Relying on `openai-go` defaults. Some proxies require explicit `Content-Type`.

**Fix**: Explicitly set headers in the OpenAI client options.

### 17. `PollUntilDone` uses fixed 2-second interval

**Location**: `internal/workflow/poll.go:10`
**Problem**: Polls Postgres every 2 seconds regardless of workflow duration. For short workflows this is fine; for long ones (hours at 100x), it's wasted queries.

**Fix**: Implement exponential backoff: 1s → 2s → 4s → 8s → capped at 30s.

### 18. No structured error types

**Location**: Throughout
**Problem**: All errors are string-wrapped with `fmt.Errorf`. No typed errors for retryable vs. non-retryable, connection errors, etc.

**Fix**: Define sentinel errors: `ErrRetryable`, `ErrConnection`, `ErrRateLimited`, etc.

### 19. Preprocessor `errgroup` uses `context.Background()` unconditionally

**Location**: `internal/preprocessor/preprocessor.go:100`
**Problem**: `errgroup.WithContext(context.Background())` ignores any parent context. If the CLI is cancelled, preprocessor keeps running.

**Fix**: Accept a `ctx` parameter in `ProcessAllFiles` and use `errgroup.WithContext(ctx)` instead.

### 20. Memory store never garbage-collects conversations

**Location**: `internal/memory/memory.go`
**Problem**: `RingBuffer` stores conversations in a `map[string][]Turn` that grows forever. No TTL or cleanup mechanism.

**Fix**: Add a periodic cleanup goroutine or LRU eviction. Acceptable for now since it's in-memory only, but document the leak.
