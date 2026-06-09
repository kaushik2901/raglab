# Eval JSONL Streaming — Implementation Plan

## Why

The current eval CLI reads all `.json` files from a directory, validates/unmarshals each entirely into memory, then creates one River job per file. This doesn't scale:

- Memory grows linearly with total question count
- River job count grows linearly with file count
- Directory scanning + per-file validation adds CLI complexity

Switching to a single `.jsonl` file with streaming mini-batches fixes all three:

- **Memory**: capped at `batchSize` questions in flight (default 20)
- **Jobs**: one River job regardless of dataset size
- **File format**: one question per JSON line, no wrapper struct
- **Validation**: lazy — skip malformed lines with a warning

---

## File Format

**Input**: `dataset.jsonl` — one JSON object per line, UTF-8.

```jsonl
{"id":"q1","question":"What is CI/CD?","category":"devops","difficulty":"easy","expected_answer":"...","relevance":[{"document_path":"ci-cd.md","grade":3}]}
{"id":"q2","question":"How to set up runners?","category":"devops","difficulty":"medium","expected_answer":"...","relevance":[{"document_path":"runners.md","grade":3}]}
```

Each line is a `types.EvalQuestion`. No wrapping `EvalDataset`/`EvalDatasetMeta`.

Validation is done lazily in the worker — if a line can't be unmarshalled, log a warning and skip it. No upfront validation pass.

---

## Architecture: Three-Stage Pipeline with Channels

```
                    ┌──────────────┐
                    │  JSONL File  │
                    └──────┬───────┘
                           │ scanner.Read()
                           ▼
              ┌───────────────────────┐
              │     Publisher         │
              │  reads line by line,  │
              │  parses EvalQuestion, │
              │  sends to questionCh  │
              │                       │
              │  backpressure: waits  │
              │  if channel is full   │
              └─────────┬─────────────┘
                        │ questionChan (buffered, size = batchSize)
              ┌─────────▼─────────────┐
              │     Embedder (x1)     │
              │  Gathers batchSize    │
              │  questions, batch     │
              │  embeds them, sends   │
              │  to workChan          │
              └─────────┬─────────────┘
                        │ workChan (buffered, size = batchSize)
              ┌─────────▼─────────────┐
              │  Fan-out to M workers │
              │  (goroutine pool)     │
              └───────────────────────┘
                        │
              ┌─────────▼─────────────┐
              │  Subscriber (x M)     │
              │  1. Search Qdrant     │
              │  2. Generate answer   │
              │  3. Judge answer      │
              │  4. Send to resultCh  │
              └─────────┬─────────────┘
                        │ resultChan (buffered, size = batchSize)
              ┌─────────▼─────────────┐
              │  Collector            │
              │  receives results,    │
              │  appends to local     │
              │  every batchSize:     │
              │  BulkAddQueryResults  │
              │  update checkpoint    │
              └─────────┬─────────────┘
                        │
              ┌─────────▼─────────────┐
              │  Aggregator (end)     │
              │  ComputeAggregateMetrics
              │  UpdateRunMetrics     │
              └───────────────────────┘
```

### Why a dedicated embed goroutine?

The naive approach (each subscriber individually embeds) destroys API efficiency:
- Per-question embedding = `N` API calls instead of `N/batchSize`
- M subscribers each making parallel embed calls increases rate-limit contention

A single embed goroutine preserves the existing batch embedding strategy:
1. Gathers `batchSize` questions from `questionChan`
2. One `emb.Embed()` call
3. Sends each `(question, embedding)` pair to `workChan`
4. M subscribers read from `workChan`, each picks one work unit

This keeps the API call count at `N/batchSize` (unchanged from current) while still allowing parallel search+generate+judge.

### Publisher

- Opens JSONL file, creates `bufio.Scanner`
- Reads one line, parses into `EvalQuestion`
- Sends to `questionChan` (buffered, size = `batchSize`)
- **Backpressure**: channel send blocks when full — publisher waits
- Propagates `scanner.Err()` via a dedicated error channel

### Embedder

- Single goroutine, reads `batchSize` questions from `questionChan`
- Batch embeds via `emb.Embed()`
- Sends `workUnit{Question: q, Embedding: vec}` for each to `workChan`
- When `questionChan` closes, flushes final partial batch then closes `workChan`

### Subscribers (M workers)

- M configurable via `--workers` (default = runtime.GOMAXPROCS(0))
- Each reads from `workChan`, processes one work unit:
  1. Search Qdrant with embedding
  2. Generate answer
  3. Judge answer
  4. Send `RetrievalResult` to `resultChan`
- 2-minute timeout per question (same as current)
- Token bucket rate limiter shared across all subscribers

### Collector

- Reads from `resultChan`
- Appends to local `[]RetrievalResult` slice
- Every `batchSize` results: call `BulkAddQueryResults` + update checkpoint
- On completion: compute aggregate metrics, `UpdateRunMetrics`

---

## Goroutine Coordination (Errgroup)

All goroutines managed by `errgroup.Group` with context propagation:

```go
g, ctx := errgroup.WithContext(ctx)

// Publisher
g.Go(func() error { return publish(ctx, file, questionChan, &lineCount, checkpoint) })

// Embedder
g.Go(func() error { return embedLoop(ctx, args, questionChan, workChan) })

// Subscribers
for i := 0; i < args.Workers; i++ {
    g.Go(func() error { return subscriberLoop(ctx, args, workChan, resultChan) })
}

// Collector
var allResults []types.RetrievalResult
g.Go(func() error { return collect(ctx, resultChan, &allResults, ...) })

err := g.Wait()
```

If any goroutine returns an error (e.g., API failure, file I/O error), the context is cancelled and all goroutines exit. Publisher can send errors to a dedicated `errCh` that the collector watches alongside `resultChan`.

---

## Checkpoint

- After each `BulkAddQueryResults` flush, call `JobUpdate` with `{lines_processed: N}` where N is the total number of successfully processed lines so far
- On retry: read output checkpoint, skip that many lines in the JSONL file
- Duplicate handling: before starting the publisher, delete any existing results for this eval run ID from `eval_queries`. If this is a retry, the checkpoint ensures we don't re-process, but the delete handles the case where a partial batch was flushed but not checkpointed.

```go
if linesToSkip > 0 {
    // On retry: clear previously stored results for this run
    w.EvalStore.DeleteRunResults(ctx, evalRunID)
}
```

Add `DeleteRunResults` to `EvalStore`:

```go
func (s *EvalStore) DeleteRunResults(ctx context.Context, runID string) error {
    _, err := s.pool.Exec(ctx, `DELETE FROM eval_queries WHERE run_id = $1`, runID)
    return err
}
```

---

## Aggregate Metrics

Computed once at the very end after all questions are processed and all results are collected:

```go
aggregate := eval.ComputeAggregateMetrics(allResults, ks)
```

This accumulates all results in memory. For 10k questions × 2KB = 20MB — acceptable for offline batch. If needed later, metrics like HitRate/MRR can be computed incrementally per question (they're just averages), but that optimization is premature now.

`ks` becomes configurable via `--ks` flag (e.g. `--ks 1,3,5,10`).

---

## Files to Create / Modify / Delete

### Modify: `cmd/eval/main.go`

| Change | Detail |
|--------|--------|
| Flag | `--dataset-dir` → `--dataset <path>` (single .jsonl file) |
| Flag | Add `--ks` (comma-separated list of K values, default `1,3,5,10`) |
| Flag | Add `--workers` (number of concurrent evaluators, default CPU count) |
| Logic | Remove directory scan, file listing, validation loop, multi-job insert, errgroup |
| Insert | Single `riverClient.Insert(...)` + single `PollUntilTerminal` |
| Import | Remove `bytes`, `filepath`, `strings`, `errgroup`, `types`, `rivertype` |

### Modify: `internal/workflow/eval_worker.go`

| Change | Detail |
|--------|--------|
| `EvalArgs` | Remove `MainTag`. Add `Ks []int`, `Workers int` |
| Dataset | `os.Open` + `bufio.NewScanner` instead of `os.ReadFile` |
| Pipeline | Three-stage channel pipeline with dedicated embed goroutine |
| Checkpoint | `JobUpdate(Output)` after each batch flush |
| Aggregation | `ComputeAggregateMetrics` after all results collected |
| Store | `BulkAddQueryResults` per-batch, `UpdateRunMetrics` at end |

### Add: `internal/eval/worker.go`

A standalone `EvaluateQuestion` function extracted from the current pipeline — takes a pre-embedded question, searches Qdrant, generates, judges, returns `RetrievalResult`. Called by subscribers.

```go
func EvaluateQuestion(ctx context.Context, q types.EvalQuestion, embedding []float64, searcher VectorSearcher, gen generator.Generator, judgeGen generator.Generator, collection string, topK int) (types.RetrievalResult, error)
```

### Modify: `internal/eval/store.go`

| Change | Detail |
|--------|--------|
| Add | `DeleteRunResults(ctx, runID)` method |

### Modify: `internal/types/eval.go`

| Change | Detail |
|--------|--------|
| Keep | `EvalQuestion`, `RelevanceJudgment`, `RetrievalResult`, `AggregateMetrics` |
| Remove | `EvalDataset` struct |
| Remove | `EvalDatasetMeta` struct |

### Delete: `internal/types/types_test.go`

Remove `TestEvalDatasetCreation` test.

---

## Memory Profile

| Component | Memory |
|-----------|--------|
| File scanner | 1 line buffer (~1KB) |
| questionChan | `batchSize` × `EvalQuestion` (~20KB) |
| Embedder batch buffer | `batchSize` × `Chunk` (~20KB) |
| Embedding vectors in flight | `batchSize` × vector (~160KB for 1536-dim) |
| workChan | `batchSize` × workUnit (~180KB) |
| resultChan | `batchSize` × `RetrievalResult` (~40KB) |
| allResults accumulator | grows linearly — all N results (20MB for 10k) |
| **Total steady state** | ~420KB + allResults at end |

---

## CLI Flag Changes

| Flag | Old | New |
|------|-----|-----|
| `--dataset-dir` | Required, directory | **Removed** |
| `--dataset` | — | Required, path to `.jsonl` file |
| `--ks` | — | Comma-separated K values (default `1,3,5,10`) |
| `--workers` | — | Concurrent evaluator count (default CPU count) |
| `--batch-size` | Unchanged | Questions per embed batch |
| `--index-tag` | Unchanged | Qdrant collection name |
| `--top-k` | Unchanged | Search result count |

---

## Verification

1. Create `testdata/eval/sample.jsonl` with 5-10 questions
2. `go build -o bin\eval.exe .\cmd\eval`
3. `.\bin\eval.exe --index-tag <tag> --query-strategy naive-search --dataset testdata/eval/sample.jsonl`
4. Verify `eval_runs` + `eval_queries` tables have correct data
5. Kill worker mid-way, restart, verify it skips processed lines via checkpoint
6. `go test ./internal/eval ./internal/workflow ./internal/types`
