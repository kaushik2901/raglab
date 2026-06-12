# Eval Run Configuration Display — Implementation Plan

## Goal

Show full evaluation run metadata in the UI (embedding model, judge model, K values, batch size, workers, dataset path) — both in the run list and the run detail page.

---

## Phase 1: Backend — Store Full Strategy

### 1.1 Expand stored strategy in eval worker (`internal/workflow/eval_worker.go`)

Current `CreateRun` call (around line 109):

```go
evalRunID, err := w.EvalDB.CreateRun(ctx, args.Tag, map[string]any{
    "index_tag":      args.IndexTag,
    "query_strategy": args.QueryStrategy,
    "llm_provider":   args.LLMProvider,
    "llm_model":      args.LLMModel,
    "judge_provider": args.JudgeProvider,
    "judge_model":    args.JudgeModel,
})
```

Replace with:

```go
strategy := map[string]any{
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
}
evalRunID, err := w.EvalDB.CreateRun(ctx, args.Tag, strategy)
```

**No DB migration needed** — `strategy` column is already `JSONB NOT NULL DEFAULT '{}'`. New fields are simply additional keys in the JSON object.

**Existing runs**: Old runs stored with the minimal set will display `—` for missing fields in the UI (handled gracefully).

### 1.2 Verify API response

The `GET /api/v1/eval/runs/{id}` endpoint uses `RunSummary` which serializes the `strategy` column as `map[string]any`. The new keys will automatically appear in the response JSON without any code changes.

Example response after change:

```json
{
  "data": {
    "id": "uuid",
    "tag": "my-eval",
    "strategy": {
      "index_tag": "handbook-v2",
      "query_strategy": "naive-search",
      "ks": [1, 3, 5],
      "llm_provider": "openai",
      "llm_model": "gpt-4o-mini",
      "embedding_provider": "openai",
      "embedding_model": "text-embedding-3-small",
      "judge_provider": "openai",
      "judge_model": "gpt-4o-mini",
      "batch_size": 20,
      "workers": 5,
      "dataset_path": "eval-set-v1"
    },
    "metrics": { ... },
    "question_count": 42,
    "created_at": "...",
    "total": 42,
    "questions": [ ... ]
  }
}
```

---

## Phase 2: Frontend — Typed Strategy Interface

### 2.1 Update types (`web/src/api/types.ts`)

Add a typed interface for the strategy object:

```typescript
export interface EvalStrategy {
  index_tag?: string
  query_strategy?: string
  ks?: number[]
  llm_provider?: string
  llm_model?: string
  embedding_provider?: string
  embedding_model?: string
  judge_provider?: string
  judge_model?: string
  batch_size?: number
  workers?: number
  dataset_path?: string
}
```

Update `RunSummary` to use the typed interface:

```typescript
export interface RunSummary {
  id: string
  tag: string
  strategy: EvalStrategy
  metrics: Record<string, unknown> | null
  question_count: number
  created_at: string
}
```

All fields are optional (`?`) to handle old runs that only have the minimal set.

### 2.2 Update `RunDetail` type (if needed)

`RunDetail extends RunSummary` — no additional changes needed since the strategy type propagates.

---

## Phase 3: RunList — Add Strategy Columns

### 3.1 Current layout (`web/src/pages/Evaluations/RunList.tsx`)

Existing columns: Tag, Questions, Created, Actions.

Add columns between Questions and Created:

| Column | Source | Format |
|---|---|---|
| LLM Model | `run.strategy.llm_model` | `font-mono text-xs` |
| Strategy | `run.strategy.query_strategy` | Badge with label mapping |

### 3.2 Strategy label mapping

Add a reusable label map (shared or duplicated in both RunList and RunDetail):

```typescript
const STRATEGY_LABELS: Record<string, string> = {
  "naive-search": "Naive Search (vector similarity)",
  "mmr-rerank": "MMR Re-rank (diversity-aware)",
  "no-retrieval": "No Retrieval (LLM baseline)",
}
```

### 3.3 Table header and cell changes

```tsx
<TableHead className="w-36">LLM Model</TableHead>
<TableHead className="w-40">Strategy</TableHead>
```

```tsx
<TableCell className="font-mono text-xs text-muted-foreground">
  {run.strategy?.llm_model ?? "—"}
</TableCell>
<TableCell>
  <Badge variant="outline" className="text-xs font-normal">
    {STRATEGY_LABELS[run.strategy?.query_strategy ?? ""] ?? run.strategy?.query_strategy ?? "—"}
  </Badge>
</TableCell>
```

### 3.4 Responsive handling

On small screens, these additional columns should collapse or scroll horizontally. The existing table uses shadcn's `Table` which handles overflow with horizontal scroll.

---

## Phase 4: RunDetail — Add Configuration Card

### 4.1 New "Run Configuration" card

Insert between the metrics cards and the charts section in `RunDetail.tsx`:

```tsx
{run.strategy && Object.keys(run.strategy).length > 0 && (
  <Card>
    <CardHeader className="pb-3">
      <div className="flex items-center gap-2">
        <RiSettingsLine className="size-4 text-muted-foreground" />
        <CardTitle className="text-sm font-medium">Run Configuration</CardTitle>
      </div>
    </CardHeader>
    <CardContent>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-x-6 gap-y-3 text-sm">
        <ConfigField label="Index Tag" value={strategy.index_tag} />
        <ConfigField label="Query Strategy" value={strategy.query_strategy} format={formatStrategy} />
        <ConfigField label="K Values" value={strategy.ks} format={v => v?.join(", ")} />
        <ConfigField label="LLM Provider" value={strategy.llm_provider} />
        <ConfigField label="LLM Model" value={strategy.llm_model} />
        <ConfigField label="Embedding Provider" value={strategy.embedding_provider} />
        <ConfigField label="Embedding Model" value={strategy.embedding_model} />
        <ConfigField label="Judge Provider" value={strategy.judge_provider} />
        <ConfigField label="Judge Model" value={strategy.judge_model} />
        <ConfigField label="Batch Size" value={strategy.batch_size} />
        <ConfigField label="Workers" value={strategy.workers} />
        <ConfigField label="Dataset Path" value={strategy.dataset_path} />
      </div>
    </CardContent>
  </Card>
)}
```

### 4.2 ConfigField helper component

Inline or extracted:

```tsx
function ConfigField({ label, value, format }: {
  label: string
  value: unknown
  format?: (v: any) => string
}) {
  if (value == null) return null
  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{label}</p>
      <p className="mt-0.5 font-mono text-sm">{format ? format(value) : String(value)}</p>
    </div>
  )
}
```

### 4.3 Provider logos / badges

Optionally render a small badge or icon next to provider names for visual scanability.

---

## Phase 5: RunCompare — Include Config Sidebar

### 5.1 Compare page (`web/src/pages/Evaluations/RunCompare.tsx`)

The compare page shows metrics side-by-side. Add strategy summary below each run's header:

```tsx
<div className="text-xs text-muted-foreground space-y-1">
  <p>LLM: {run.strategy.llm_model ?? "—"}</p>
  <p>Embed: {run.strategy.embedding_model ?? "—"}</p>
  <p>Judge: {run.strategy.judge_model ?? "—"}</p>
  <p>Strategy: {STRATEGY_LABELS[run.strategy.query_strategy ?? ""] ?? run.strategy.query_strategy ?? "—"}</p>
  <p>K: {run.strategy.ks?.join(", ") ?? "—"}</p>
</div>
```

---

## Phase 6: Edge Cases & Polish

- **Old runs** with minimal strategy: all new fields will be `undefined` → `ConfigField` renders `null` (hidden) since value is `null`/`undefined`
- **Null strategy**: Guard with `run.strategy && Object.keys(run.strategy).length > 0` — if the strategy is `{}` or `null`, the whole card is hidden
- **Array values** (`ks`): Use `format` function to join into readable string: `[1, 3, 5]` → `"1, 3, 5"`
- **Long values** (`dataset_path`): Truncate with ellipsis and show full value in tooltip
- **Empty state messaging**: In the configuration card, if only partial data exists, missing fields are simply omitted (not shown as "—") to reduce noise

---

## Files Created / Modified

| Action | Path |
|---|---|
| MODIFY | `internal/workflow/eval_worker.go` (expand strategy map) |
| MODIFY | `web/src/api/types.ts` (add `EvalStrategy` interface) |
| MODIFY | `web/src/pages/Evaluations/RunList.tsx` (add LLM + Strategy columns) |
| MODIFY | `web/src/pages/Evaluations/RunDetail.tsx` (add Configuration card) |
| MODIFY | `web/src/pages/Evaluations/RunCompare.tsx` (add config summary) |
