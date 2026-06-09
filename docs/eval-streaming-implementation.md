# Eval Streaming — Phase-wise Implementation Plan

## Overview

Replace the current eval CLI (multi-file, all-in-memory, multi-job) with a single-jsonl streaming pipeline using goroutines and channels. Memory stays bounded by `batchSize`. M workers evaluate questions in parallel.

---

## Phase 1: Types + Store Foundation

No behavioral changes. Pure cleanup + new primitives.

### 1a. Remove `EvalDataset` / `EvalDatasetMeta` from `internal/types/eval.go`

**File**: `internal/types/eval.go`

Remove these structs:

```go
type EvalDataset struct { ... }        // DELETE
type EvalDatasetMeta struct { ... }    // DELETE
```

Keep everything else: `EvalQuestion`, `RelevanceJudgment`, `RetrievalResult`, `AggregateMetrics`.

### 1b. Remove `TestEvalDatasetCreation` from `internal/types/types_test.go`

**File**: `internal/types/types_test.go`

Delete the function `TestEvalDatasetCreation` (lines ~102-111 referencing `EvalDataset{...}`).

### 1c. Add `DeleteRunResults` to `EvalStore`

**File**: `internal/eval/store.go`

```go
func (s *EvalStore) DeleteRunResults(ctx context.Context, runID string) error {
    _, err := s.pool.Exec(ctx, `DELETE FROM eval_queries WHERE run_id = $1`, runID)
    if err != nil {
        return fmt.Errorf("delete run results: %w", err)
    }
    return nil
}
```

### 1d. Add `EvaluateQuestion` to `internal/eval/worker.go` (new file)

**File**: `internal/eval/worker.go` (create)

Extract the per-question logic from `internal/eval/pipeline.go:50-88` into a standalone function. Takes a pre-embedded question, searches Qdrant, generates, judges, returns `RetrievalResult`.

```go
package eval

func EvaluateQuestion(
    ctx context.Context,
    q types.EvalQuestion,
    embedding []float64,
    searcher VectorSearcher,
    gen generator.Generator,
    judgeGen generator.Generator,
    collection string,
    topK int,
) (types.RetrievalResult, error) {
    // 1. Search Qdrant with the embedding
    // 2. Fill RetrievalResult (fillRetrievalResult)
    // 3. Generate answer if generator provided
    // 4. Judge answer if judge provided
    // 5. Return result
}
```

Also move `fillRetrievalResult`, `buildContextText`, `generateForQuestion` helpers from `pipeline.go` into `worker.go` (they're only needed here).

**The original `Evaluate` in `pipeline.go` is kept for now** — it will be removed in Phase 4.

### 1e. Add `DeleteRunResults` test to `internal/eval/store_test.go`

```go
func TestEvalStore_DeleteRunResults(t *testing.T) {
    // create run, add results, delete, verify empty
}
```

---

## Phase 2: CLI Simplification

**File**: `cmd/eval/main.go`

Replace the multi-file, multi-job CLI with a single-job insert.

### 2a. Flag changes

| Flag | Change |
|------|--------|
| `--dataset-dir` | **Remove** |
| `--dataset` | **Add** — path to single `.jsonl` file |
| `--ks` | **Add** — comma-separated K values, default `1,3,5,10` |
| `--workers` | **Add** — concurrent evaluators, default `5` |

### 2b. Remove entire directory scan + validation block

Delete:
- `absDir`, `relDir` resolution and path manipulation
- `os.ReadDir` + file listing loop
- `validateDatasetFile()` function
- `filepath` and `strings` imports (no longer used)

### 2c. Remove multi-job insert + errgroup polling

Replace the `fileJob` struct, `for` loop over files, `riverClient.Insert` per file, and errgroup `PollUntilTerminal` with a single:

```go
result, err := rc.Client.Insert(ctx, &workflow.EvalArgs{...}, nil)
workflow.PollUntilTerminal(ctx, rc.Client, result.Job.ID, 2*time.Second)
```

### 2d. Updated EvalArgs passed to worker

```go
Tag               string   `json:"tag"`
IndexTag          string   `json:"index_tag"`
QueryStrategy     string   `json:"query_strategy"`
DatasetPath       string   `json:"dataset_path"`
TopK              int      `json:"top_k"`
Ks                []int    `json:"ks"`
LLMProvider       string   `json:"llm_provider"`
LLMModel          string   `json:"llm_model"`
EmbeddingProvider string   `json:"embedding_provider"`
EmbeddingModel    string   `json:"embedding_model"`
JudgeProvider     string   `json:"judge_provider"`
JudgeModel        string   `json:"judge_model"`
Workers           int      `json:"workers"`
BatchSize         int      `json:"batch_size"`
```

Removed `MainTag`, `Concurrency`, `WorkflowID`.

---

## Phase 3: Worker Rewrite (Streaming Pipeline)

**File**: `internal/workflow/eval_worker.go`

Full rewrite of the `Work` method. This is the core change.

### 3a. Define workUnit type

```go
type workUnit struct {
    LineNum   int
    Question  types.EvalQuestion
    Embedding []float64
}
```

### 3b. State types

```go
type evalCheckpoint struct {
    LinesProcessed int `json:"lines_processed"`
}
```

### 3c. Pipeline stages

#### Stage 1 — Publisher (`publishLines`)

```
Input:  file handle (os.File)
Output: questionChan (chan types.EvalQuestion)
```

- Creates `bufio.Scanner` with 1MB line buffer
- Counts lines, skips up to `checkpoint.LinesProcessed`
- Parses each line into `EvalQuestion`
- Validates: `id != ""` and `question != ""`
- Sends to `questionChan` (blocks if full — backpressure)
- On `scanner.Err()` or context cancel: return error
- On EOF: close `questionChan`, return nil

#### Stage 2 — Embedder (`embedBatch`)

```
Input:  questionChan (chan types.EvalQuestion)
Output: workChan (chan workUnit)
```

- Accumulates `batchSize` questions from `questionChan`
- Calls `emb.Embed()` with the batch
- Sends one `workUnit{lineNum, question, embedding}` per question to `workChan`
- When `questionChan` closes: flush final partial batch, close `workChan`

**Line number tracking**: The embedder tracks a running line counter. Each line read from `questionChan` increments it. This counter is included in each `workUnit` for checkpoint alignment.

#### Stage 3 — Subscribers (M workers, `evalQuestion`)

```
Input:  workChan (chan workUnit)
Output: resultChan (chan types.RetrievalResult)
```

- Reads one `workUnit` from `workChan`
- Calls `eval.EvaluateQuestion()` with the pre-embedded question
- Sends result to `resultChan`
- 2-minute timeout per question via question-level context
- Runs until `workChan` closes, then exits

#### Stage 4 — Collector (`collectResults`)

```
Input:  resultChan (chan types.RetrievalResult)
Output: allResults ([]types.RetrievalResult) + DB writes
```

- Listens on both `resultChan` and `ctx.Done()`
- Appends each result to `allResults`
- Every `batchSize` results:
  1. `BulkAddQueryResults` the batch
  2. `JobUpdate(Output: {lines_processed: lastCheckpointedLine})`
- When `resultChan` closes:
  1. `BulkAddQueryResults` final partial batch
  2. Compute aggregate metrics via `ComputeAggregateMetrics(allResults, args.Ks)`
  3. `UpdateRunMetrics`

### 3d. Work function structure

```go
func (w *EvalWorker) Work(ctx context.Context, job *river.Job[EvalArgs]) error {
    args := job.Args

    // 1. Read checkpoint
    cp := readEvalCheckpoint(job)

    // 2. Create eval run
    evalRunID, err := w.EvalStore.CreateRun(ctx, args.Tag, strategyMap)
    // On retry: clear any previous results for this run
    if cp.LinesProcessed > 0 {
        w.EvalStore.DeleteRunResults(ctx, evalRunID)
    }

    // 3. Create deps (embedder, qdrant, generator, judge)
    emb, qStore, gen, judgeGen := createEvalDeps(ctx, args)
    defer qStore.Close()

    // 4. Open file
    file, err := os.Open(args.DatasetPath)

    // 5. Create channels
    questionChan := make(chan types.EvalQuestion, args.BatchSize)
    workChan := make(chan workUnit, args.BatchSize)
    resultChan := make(chan types.RetrievalResult, args.BatchSize)
    defer close(questionChan) // safety: ensures embedder unblocks if publisher errors

    // 6. Errgroup
    var allResults []types.RetrievalResult
    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error {
        defer close(questionChan)
        return publishLines(ctx, file, questionChan, cp.LinesProcessed)
    })
    g.Go(func() error {
        defer close(workChan)
        return embedBatch(ctx, args, emb, questionChan, workChan)
    })
    for i := 0; i < args.Workers; i++ {
        g.Go(func() error {
            return evalQuestion(ctx, args, qStore, gen, judgeGen, workChan, resultChan)
        })
    }
    g.Go(func() error {
        return collectResults(ctx, args, w.EvalStore, w.Client, job, evalRunID, resultChan, &allResults)
    })

    return g.Wait()
}
```

### 3e. Checkpoint helpers

```go
func readEvalCheckpoint(job *river.Job[EvalArgs]) evalCheckpoint {
    raw := job.Output()
    if len(raw) == 0 { return evalCheckpoint{} }
    var cp evalCheckpoint
    json.Unmarshal(raw, &cp)
    return cp
}

func saveEvalCheckpoint(ctx context.Context, client *river.Client[pgx.Tx], job *river.Job[EvalArgs], linesProcessed int) error {
    _, err := client.JobUpdate(ctx, job.ID, &river.JobUpdateParams{
        Output: evalCheckpoint{LinesProcessed: linesProcessed},
    })
    return err
}
```

### 3f. Error handling

- `errgroup` context cancellation propagates to all stages
- Each channel read/write uses `select` with `<-ctx.Done()` to avoid hangs on shutdown
- Publisher returns `scanner.Err()` if non-nil
- Embedder returns error if `emb.Embed()` fails
- Subscribers return nil (errors are per-question; failed questions get `Failed: true` in results)
- Collector returns error if `BulkAddQueryResults` or `UpdateRunMetrics` fail

---

## Phase 4: Cleanup

### 4a. Remove `Evaluate` from `internal/eval/pipeline.go`

Delete the `Evaluate` function and `batchEmbedQueries` — they're replaced by the channel-based pipeline. Keep `VectorSearcher` interface (used by `EvaluateQuestion`).

Move the relevant helpers (`fillRetrievalResult`, `buildContextText`, `generateForQuestion`) to `worker.go` where `EvaluateQuestion` lives.

**File**: `internal/eval/pipeline.go`

After removing `Evaluate` and `batchEmbedQueries`, the file only contains:
- `PipelineArgs` struct (can keep or remove — check if used elsewhere)
- `Context` helpers if any

Actually, `PipelineArgs` was only used by `Evaluate`. Both can go. The file can be deleted entirely after moving `fillRetrievalResult`, `buildContextText`, `generateForQuestion` to `worker.go`.

### 4b. Update imports

- `cmd/eval/main.go`: remove `bytes`, `filepath`, `strings`, `types`
- `internal/workflow/eval_worker.go`: clean unused imports

### 4c. Update tests

- `internal/eval/pipeline_test.go`: If `Evaluate` was tested, remove those tests (or rewrite for `EvaluateQuestion`)
- `internal/workflow/eval_worker_test.go`: Update to match new `EvalArgs` struct

---

## Phase Order

```
Phase 1: Types + Store Foundation
    → internal/types/eval.go           (remove EvalDataset, EvalDatasetMeta)
    → internal/types/types_test.go      (remove TestEvalDatasetCreation)
    → internal/eval/store.go            (add DeleteRunResults)
    → internal/eval/store_test.go       (add DeleteRunResults test)
    → internal/eval/worker.go (NEW)     (add EvaluateQuestion + helpers)

Phase 2: CLI Simplification
    → cmd/eval/main.go                  (simplify to single-job)

Phase 3: Worker Rewrite
    → internal/workflow/eval_worker.go  (streaming pipeline)

Phase 4: Cleanup
    → internal/eval/pipeline.go         (remove Evaluate, batchEmbedQueries)
    → Updated imports / tests
```

Each phase produces a buildable, testable state. Verify after each phase with `go build ./...` and `go test ./...`.
