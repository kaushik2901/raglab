# No-Retrieval Baseline Strategy — Implementation Plan

## Goal

Add a `no-retrieval` query strategy that evaluates the LLM without any document context, establishing a baseline to measure RAG's value-add.

---

## Phase 1: Register the Strategy

### 1.1 New file (`internal/retriever/no_retrieval.go`)

```go
package retriever

import (
    "context"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func init() {
    RegisterRetriever("no-retrieval", noRetrievalFactory)
}

func noRetrievalFactory(_ store.VectorStore) RetrieveFunc {
    return func(_ context.Context, _ string, _ []float32, _ int) ([]types.SearchResult, error) {
        return []types.SearchResult{}, nil
    }
}
```

- Registers in `init()` so it's available at startup alongside `naive-search` and `mmr-rerank`
- Factory ignores the `VectorStore` argument entirely
- Returns an empty slice — zero results, no documents to inject into context

### 1.2 Verify registration

No changes needed to `retriever/retriever.go` — the `init()` pattern is already established. The `New(store, strategy)` function will validate `"no-retrieval"` against the registry map.

---

## Phase 2: Adapt the Eval Worker

### 2.1 Understand the eval loop (`internal/workflow/eval_worker.go`)

Read the full `Work()` method to identify the embedding and retrieval call sites. The current flow:

1. Parse args → create eval run → open dataset
2. **Batch embed** all questions (call embedder)
3. **Worker pool loop**: for each question → search → generate answer → judge score
4. Store results → update metrics

When `no-retrieval`:
- Step 2 (batch embed) is unnecessary — skip entirely
- Step 3's search/retrieval becomes a no-op (empty results from the strategy)
- Step 3's generate must omit document context from the prompt

### 2.2 Code changes in `Work()` method

**Before batch embedding (around lines 120-140):**

```go
if args.QueryStrategy != "no-retrieval" {
    // existing batch embed logic
} else {
    slog.Info("no-retrieval strategy: skipping embedding, answering without context")
}
```

- The `workUnit` struct has an `Embedding` field — when `no-retrieval`, leave it as zero/nil
- The retriever call (`w.Retriever.Search(...)`) will return empty results since registered strategy returns `[]`

**Generate answer with no context:**

In the `generateAnswer` function (or equivalent inline code), when the search results slice is empty, construct a prompt without context documents:

```
Prompt with context:     "Answer based on these documents:\n{docs}\n\nQuestion: {question}"
Prompt without context:  "Answer the question:\n{question}"
```

Check how the prompt template is built — likely via `generator.Generate(ctx, prompt)` or similar. If the prompt template is hardcoded with context insertion, modify it to conditionally skip context when results are empty.

### 2.3 Store full strategy metadata

In `CreateRun` call (around line 109), ensure all `EvalArgs` fields are stored:

```go
evalRunID, err := w.EvalDB.CreateRun(ctx, args.Tag, map[string]any{
    "index_tag":           args.IndexTag,
    "query_strategy":      args.QueryStrategy,
    "ks":                  args.Ks,
    "llm_provider":        args.LLMProvider,
    "llm_model":           args.LLMModel,
    "embedding_provider":  args.EmbeddingProvider,
    "embedding_model":     args.EmbeddingModel,
    "judge_provider":      args.JudgeProvider,
    "judge_model":         args.JudgeModel,
    "batch_size":          args.BatchSize,
    "workers":             args.Workers,
    "dataset_path":        args.DatasetPath,
})
```

This is needed for all strategies (not just no-retrieval). Covered in detail in the eval-config-display plan.

---

## Phase 3: Frontend Changes

### 3.1 EvalCreate.tsx — Add strategy option

```tsx
<SelectItem value="no-retrieval">No Retrieval (LLM baseline)</SelectItem>
```

### 3.2 Conditional index field behavior

When `no-retrieval` is selected:
- The Index dropdown becomes irrelevant (no vector search happens)
- Options:
  a. Hide the Index selector entirely and show a note: "No index needed — answering without document context."
  b. Keep it visible but optional (don't require it for form submission)

Option (a) is cleaner UX. Implement:

```tsx
const needsIndex = queryStrategy !== "no-retrieval"
```

Wrap index field in a conditional block. Remove `required` from validation when `no-retrieval`.

### 3.3 RunDetail.tsx — Show strategy label

The `no-retrieval` strategy should display as `"No Retrieval (LLM baseline)"` in the run detail configuration card. Add a mapping in the UI:

```typescript
const STRATEGY_LABELS: Record<string, string> = {
  "naive-search": "Naive Search (vector similarity)",
  "mmr-rerank": "MMR Re-rank (diversity-aware)",
  "no-retrieval": "No Retrieval (LLM baseline)",
}
```

---

## Phase 4: Edge Cases & Verification

- **Judge still runs**: Even without retrieval, the judge should score the LLM's answer against the expected answer — this gives a baseline answer_score
- **Metrics computation**: HitRate / MRR / NDCG will be zero for all K values (no documents retrieved). The aggregate metrics should still compute but readers should interpret them knowing there was no retrieval. Consider skipping these retrieval-based metrics in the metrics output and only reporting `AvgAnswerScore`.
- **Prompt/completion tokens**: Still tracked normally
- **Latency**: Will be lower since embedding + search are skipped — this itself is useful data
- **Smoke test**: Run a small eval dataset with `no-retrieval` and verify:
  1. No embedder calls made
  2. Search results empty
  3. Answer generated without context
  4. Metrics computed
  5. Strategy stored correctly in DB

---

## Files Created / Modified

| Action | Path |
|---|---|
| CREATE | `internal/retriever/no_retrieval.go` |
| MODIFY | `internal/workflow/eval_worker.go` (skip embed, skip context) |
| MODIFY | `web/src/pages/Evaluations/EvalCreate.tsx` (add option + conditional index) |
| MODIFY | `web/src/pages/Evaluations/RunDetail.tsx` (strategy label mapping) |
