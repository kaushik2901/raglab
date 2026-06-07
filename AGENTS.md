# AGENTS.md — GitLab Handbook RAG Pipeline

## Commands

```powershell
go build -o bin\preprocess.exe .\cmd\preprocess   # build preprocess CLI
go build -o bin\index.exe .\cmd\index             # build index CLI
go build -o bin\query.exe .\cmd\query             # build query CLI
go build -o bin\eval.exe .\cmd\eval               # build eval CLI
go build -o bin\workerd.exe .\cmd\workerd         # build worker daemon
go test ./...                                      # all tests
Remove-Item -Recurse -Force bin,output,.journal    # clean
```

On Windows, use `make.cmd` for Docker-based builds (build/run/clean/test).  
`build.cmd` does NOT exist — use `make.cmd` or raw `go build` instead.

External Go dependencies managed via `go.mod` / `go.sum`. No `vendor/` dir.

## Pipeline Stages

| Order | Stage        | Requires            | What it does                                                                         |
| ----- | ------------ | ------------------- | ------------------------------------------------------------------------------------ |
| 1     | `clone`      | —                   | `git clone --depth 1`; if exists: `fetch --all` + `checkout main` + `pull --ff-only` |
| 2     | `preprocess` | `clone`             | Reads `{repo}/content/`, writes cleaned markdown to `--output`                       |
| 3     | `verify`     | `clone, preprocess` | Writes `_verification_report.json` to output dir                                     |

Stages are defined in `cmd/preprocess/main.go:69-72`. Use `--from <stage>` to resume.

## Quirks

- Package `internal/stage/` is named `stage` (formerly `stageimport`).
- `handbook/` is the default clone target, NOT tracked in git (but not gitignored either).
- Journal caching lives in `.journal/` (gob files per stage). Delete it to force re-run.
- `config.Config` is system-level only: `MAX_RETRIES`, `RETRY_BACKOFF`, `LOG_LEVEL`, `DATABASE_URL`, `LLM_BASE_URL`, `LLM_API_KEY`, `QDRANT_URL`, `QDRANT_API_KEY`.
- Pipeline inputs (repo URL, chunk params, etc.) are parsed inline by each `cmd/*/main.go`, not in config. Stage functions accept explicit parameters instead of `*config.Config` — see per-stage signatures.
- `go mod tidy` is only needed if adding external deps (currently none beyond existing go.mod).
- Pipeline retries use exponential backoff + jitter. `MaxRetries=0` means no retries. `RetryBackoff` must be > 0.

## Preprocessor Transform Order

1. `ResolveIncludes` — `{{% include "path" %}}` (recursive, cycle-protected)
2. `StripShortcodes` — details/alert/panel → StripTags, youtube/handbook-data-toc/member-by-\* → Remove, include/ref/relref → Resolve
3. `ProcessHTML` — strip style/script/iframe, keep img alt text, convert `<a>` to `text [url]`, flatten tables
4. `ResolveRefs` — `{{< ref "path" >}}` / `{{< relref "path" >}}` → markdown links

## Testing

- All tests use `t.TempDir()` for temp directories.
- Flag-based tests must call `resetFlags()` (sets `flag.NewFlagSet`) before each test to avoid global state conflicts.
- Preprocessor tests write files to temp dirs then run `ProcessFile` / `ProcessAllFiles`.

## Indexing Pipeline

The indexing pipeline (`cmd/index/`) builds on the preprocessing output. A single `IndexWorker` processes documents one at a time in a streaming fashion — no document content, chunks, or embeddings are stored in Postgres:

```
IndexWorker walks artifacts/preprocessing/<input_tag>/output/
  for each .md file:
    1. Read from disk (parser.ParseFile)
    2. Chunk (chunker.FixedChunker)
    3. Embed (embedder.New — any OpenAI-compatible API)
    4. Store in Qdrant (store.QdrantStore)
```

- **Types:** `internal/types/indexing.go` — `Chunk`, `Embedding`, `DocumentChunk`
- **Config:** system-level only (`MAX_RETRIES`, `RETRY_BACKOFF`, `LOG_LEVEL`, `DATABASE_URL`, `LLM_BASE_URL`, `LLM_API_KEY`, `QDRANT_URL`, `QDRANT_API_KEY`); indexing params are parsed inline in `cmd/index/main.go`
- **Parser:** `internal/parser/parser.go` — reads a single `.md` file into `types.Document`
- **Fixed chunker:** `internal/chunker/` — word-window splitting with configurable size/overlap
- **Embedder:** `internal/embedder/` — interface + OpenAI-compatible HTTP embedder with batching and rate-limit retry

## Project Vision

`docs/project-vision-and-roadmap.md` captures the full vision: durable workflows, UI/API, evaluation system, observability, and a QA chatbot. It contains detailed architecture, phase plan, and open questions.

## Key Decision: River for Workflow Engine

We use **River** (`github.com/riverqueue/river`) as the job engine — a lightweight Go queue backed by Postgres, not Temporal. River provides durable at-least-once execution, retries with backoff, and concurrency control. A thin coordination layer handles linear DAG step sequencing (each worker enqueues the next step on success). Postgres is the single source of truth for workflow state; River's internal tables are secondary.

## Phase 1: River Implementation

`docs/river-implementation-plan.md` contains the detailed implementation plan for wrapping preprocessing and indexing as durable River workflows. The plan is organized into 6 sub-phases: Postgres/River infra → workflow DB layer → preprocessing workers → indexing workers → thin CLI wrappers → journal cleanup.

## Retriever + Memory

- `internal/retriever/` — `Retriever` wraps `embedder.Embedder` + `store.VectorStore`; call `Retrieve(ctx, collection, query, topK)` for one-shot semantic search. Supports strategies (`naive-search` only currently).
- `internal/memory/` — `Memory` interface + `RingBuffer`; thread-safe per-conversation-ID ring buffer with `Add`, `Get`, `Clear`

## LLM Providers

All four providers use **OpenAI-compatible API format** under the hood. The `--llm-provider` flag selects which set of env vars to read:

| Provider     | Env API Key            | Env Base URL                      | Default Base URL                                              |
|--------------|------------------------|-----------------------------------|---------------------------------------------------------------|
| `openai`     | `OPENAI_API_KEY` → `LLM_API_KEY` | `OPENAI_BASE_URL` → `LLM_BASE_URL` | `https://api.openai.com`                    |
| `gemini`     | `GEMINI_API_KEY`       | `GEMINI_BASE_URL`                 | `https://generativelanguage.googleapis.com/v1beta/openai`     |
| `openrouter` | `OPENROUTER_API_KEY`   | `OPENROUTER_BASE_URL`             | `https://openrouter.ai/api/v1`                                |
| `lmstudio`   | *(none)*               | `LMSTUDIO_BASE_URL`               | `http://localhost:1234/v1`                                    |

### CLI flags

| Flag                     | Env                     | Default      | Applies to                        |
|--------------------------|-------------------------|--------------|-----------------------------------|
| `--llm-provider`         | `LLM_PROVIDER`          | `openai`     | All CLIs (index, eval, query)     |
| `--embedding-provider`   | `EMBEDDING_PROVIDER`    | → `--llm-provider` | index, eval, query          |
| `--judge-provider`       | `JUDGE_PROVIDER`        | → `--llm-provider` | eval only                   |

### Usage examples

```powershell
# OpenAI (default, backward-compatible)
.\bin\query.exe --llm-provider openai --llm-model gpt-4o --tag my-collection --query "..."

# OpenRouter
$env:OPENROUTER_API_KEY = "sk-or-..."
.\bin\index.exe --llm-provider openrouter --embedding-model "openai/text-embedding-3-small" --input-tag pre-20260603

# LM Studio (local, no key)
.\bin\query.exe --llm-provider lmstudio --llm-model "local-model" --tag my-collection --query "..."

# Google Gemini via OpenAI-compatible endpoint
$env:GEMINI_API_KEY = "AIza..."
.\bin\eval.exe --llm-provider gemini --llm-model "gemini-2.0-flash" --index-tag idx-fixed-512 --query-strategy naive-search --dataset-dir artifacts/...
```

### Embedder + Generator factories

- `embedder.New(provider, model, batchSize)` — resolves env vars, returns `embedder.Embedder`
- `generator.New(provider, model)` — resolves env vars, returns `generator.Generator`

Both use `config.ResolveProviderConfig(provider)` which maps provider names to env vars. Backward compat: `openai` provider falls back to `LLM_API_KEY` / `LLM_BASE_URL`.

### Rate Limiting

Rate limiting is purely **reactive** — both the embedder and generator retry on 429 with: exponential backoff + jitter + `Retry-After` header respect. Generator retries up to 5 attempts with the same backoff strategy.

## Embedding Model Dimension Mismatch

When `EnsureCollection` finds an existing Qdrant collection, it returns immediately **without checking vector dimensions**. If you switch to an embedding model that produces different-sized vectors, Qdrant will reject the upsert at runtime with a dimension mismatch error. To fix:

1. Delete the Qdrant collection manually (`DROP COLLECTION` via Qdrant UI or API)
2. Or use a different `--tag` / collection name for the new index

There is currently no `--force-recreate` flag — the safe workaround is to use a unique tag per embedding model configuration.

## Important: Rebuild workerd after provider changes

CLI changes (flags, env vars) take effect immediately via `make.cmd <cmd>`, but **River workers** (`workerd.exe`) must be rebuilt and restarted separately to pick up changes to worker logic.

```powershell
go build -o bin\workerd.exe .\cmd\workerd  # rebuild worker daemon
```

## Evaluation Pipeline

Evaluation uses a **4-phase pipeline** to maximize throughput under rate limits:

| Phase | What | API calls | Batching | Rate limit impact |
|-------|------|-----------|----------|-------------------|
| 1 | **Batch embed** all queries | `total / batchSize` (e.g. 15 for 300 Qs) | ✅ 20 queries per call | Embed provider sees 15 calls instead of 300 |
| 2 | **Search** all (Qdrant, parallel) | 1 per question | ❌ | No rate limit |
| 3 | **Generate** answers (sequential) | 1 per question | ❌ | Full chat bucket, no competition |
| 4 | **Judge** answers (sequential) | 1 per question | ❌ | Full chat bucket, no competition |

Phases 2-4 run **sequentially per question** — no parallelism within each phase. The rate limiter (token bucket) is the bottleneck, not CPU, so concurrent goroutines can't improve throughput.

### Key files

| File | Role |
|------|------|
| `cmd/eval/main.go` | Thin CLI — reads `.json` dataset files, inserts River jobs, polls workflows |
| `internal/workflow/eval_worker.go` | River worker — creates embedder/generator/judge, calls pipeline |
| `internal/eval/pipeline.go` | **NEW** — phase-based `Evaluate()` (batch embed → sequential loop) |
| `internal/eval/retrieval.go` | Legacy per-question evaluator (kept for tests) |
| `internal/eval/metrics.go` | `ComputeAggregateMetrics` — HitRate, MRR, NDCG, Precision, Recall, AvgAnswerScore |
| `internal/eval/judge.go` | `JudgeAnswer` — calls LLM to score answer correctness |
| `internal/eval/store.go` | `EvalStore` — `eval_runs` / `eval_queries` CRUD |

- Usage: `.\bin\eval.exe --index-tag idx-fixed-512 --query-strategy naive-search --dataset-dir artifacts/preprocessing/pre-20260603-141651/eval-dataset`
- Run `.\bin\eval.exe --help` for all flags
- Relevance judgments use `document_path` (e.g. `handbook/travel-policy.md`)
