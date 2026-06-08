# Dynamic Per-Document Timeout — Implementation Plan

3 phases, ordered by impact. Addresses issue #3 from `known-issues-and-fixes.md`.

## Background

The index worker hardcodes a 5-minute timeout per file at `index_worker.go:223`:
```go
docCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
```

This covers the entire parse → chunk → embed → store cycle. With the streaming refactor, we no longer have `doc.Content` upfront (no `io.ReadAll`), so the old file-size-based heuristic from the known-issues doc doesn't apply directly.

---

## Phase 0: Configurable per-document timeout

**Files:**
- `internal/workflow/index_worker.go`
- `cmd/index/main.go`
- `internal/workflow/index_args.go` (if IndexArgs grows enough to warrant)

**What changes:**

Add `DOC_TIMEOUT` env var / `--doc-timeout` flag that replaces the hardcoded 5 minutes.

```go
// cmd/index/main.go
flag.Duration("doc-timeout", config.DurationEnvOrDefault("DOC_TIMEOUT", 30*time.Minute),
    "Timeout per document (parse+chunk+embed+store)")
```

```go
// internal/workflow/index_worker.go — IndexArgs
type IndexArgs struct {
    // ... existing fields ...
    DocTimeout time.Duration `json:"doc_timeout"`
}
```

```go
// In RunIndexing, replace the hardcoded timeout:
docTimeout := args.DocTimeout
if docTimeout <= 0 {
    docTimeout = 30 * time.Minute
}
// ... inside goroutine:
docCtx, cancel := context.WithTimeout(ctx, docTimeout)
```

**Env var config helper** (if `config.DurationEnvOrDefault` doesn't exist yet — add it):

```go
// internal/config/config.go
func DurationEnvOrDefault(key string, defaultVal time.Duration) time.Duration {
    if v := os.Getenv(key); v != "" {
        if d, err := time.ParseDuration(v); err == nil {
            return d
        }
    }
    return defaultVal
}
```

**Tests:**
```
TestRunIndexing_CustomTimeout
    Create IndexArgs with DocTimeout=1ns
    Point input dir at a real .md file
    Verify processFile returns context.DeadlineExceeded error
    Verify error is gracefully skipped (added to skipErrors)
```

**Verify:** `go test ./internal/workflow/ -run TestRunIndexing`

---

## Phase 1: Per-batch timeout in `embedAndStore`

**Files:**
- `internal/workflow/index_worker.go`

**What changes:**

Add a per-batch timeout inside `embedAndStore` so a single stuck API call doesn't consume the entire per-document budget.

```go
func embedAndStore(ctx context.Context, emb embedder.Embedder, qStore qstore.VectorStore, collectionName string, batch []types.Chunk) error {
    batchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
    defer cancel()

    embeddings, err := emb.Embed(batchCtx, batch)
    if err != nil {
        return fmt.Errorf("embed batch: %w", err)
    }
    // ...
    if err := qStore.Store(batchCtx, collectionName, docChunks); err != nil {
        return fmt.Errorf("store batch: %w", err)
    }
    return nil
}
```

The batch timeout (5 min) is deliberately much shorter than the per-document timeout (default 30 min) so a single bad batch doesn't consume the whole file's budget.

**Tests:**
```
TestEmbedAndStore_ContextCancel
    Create a context that's already cancelled
    Verify embedAndStore returns context.Canceled

TestEmbedAndStore_BatchTimeout
    Create a context with very short timeout
    Verify timeout propagates correctly
```

**Verify:** `go test ./internal/workflow/ -run TestEmbedAndStore`

---

## Phase 2: File size pre-check

**Files:**
- `internal/workflow/index_worker.go`

**What changes:**

Before calling `processFile`, stat the file and skip if it exceeds a max size. This catches unreasonably large files before they waste any API calls.

```go
const maxIndexFileSize = 100 * 1024 * 1024 // 100 MB

// In RunIndexing goroutine, before processFile:
fi, err := os.Stat(fp)
if err != nil {
    mu.Lock()
    skipErrors = append(skipErrors, relPath+": stat: "+err.Error())
    mu.Unlock()
    return nil
}
if fi.Size() > maxIndexFileSize {
    mu.Lock()
    skipErrors = append(skipErrors, relPath+": file too large ("+strconv.FormatInt(fi.Size(), 10)+" bytes, max 100MB)")
    mu.Unlock()
    return nil
}
```

Make `maxIndexFileSize` configurable via `MAX_INDEX_FILE_SIZE` env var (in bytes).

**Tests:**
```
TestRunIndexing_SkipsLargeFiles
    Create a large .md file and a small .md file
    Verify the large file is skipped with a warning
    Verify the small file is processed normally
```

**Verify:** `go test ./internal/workflow/ -run TestRunIndexing`

---

## Dependency graph

```
Phase 0 (configurable timeout) ──► Phase 1 (per-batch timeout)
                                        │
                                        └──► Phase 2 (file size pre-check)
```

Phase 1 builds on Phase 0's timeout infrastructure. Phase 2 is independent and can be done in parallel with Phase 1.

---

## Testing strategy summary

| Phase | What's tested | How | Key edge cases |
|-------|--------------|-----|----------------|
| 0 | Configurable timeout propagates from CLI → IndexArgs → RunIndexing | Custom timeout value verified in processFile call | Zero/negative timeout falls back to 30min default |
| 1 | Per-batch timeout | Cancelled context passed to embedAndStore | Batch timeout shorter than doc timeout |
| 2 | File size pre-check | Stat before processFile | Non-existent file, zero-size file, exact boundary |
