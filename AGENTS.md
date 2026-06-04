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
`build.cmd` does NOT exist despite README mentioning it — use `make.cmd` or raw `go build` instead.

Zero external Go dependencies (no `go.sum` yet). No `vendor/` dir.

## Pipeline Stages

| Order | Stage        | Requires            | What it does                                                                         |
| ----- | ------------ | ------------------- | ------------------------------------------------------------------------------------ |
| 1     | `clone`      | —                   | `git clone --depth 1`; if exists: `fetch --all` + `checkout main` + `pull --ff-only` |
| 2     | `preprocess` | `clone`             | Reads `{repo}/content/`, writes cleaned markdown to `--output`                       |
| 3     | `verify`     | `clone, preprocess` | Writes `_verification_report.json` to output dir                                     |

Stages are defined in `cmd/preprocess/main.go:69-72`. Use `--from <stage>` to resume.

## Quirks

- Package `internal/stage/` is named `stageimport` (not `stage`) — imports use `stagepkg "..."`.
- `handbook/` is the default clone target, NOT tracked in git (but not gitignored either).
- Journal caching lives in `.journal/` (gob files per stage). Delete it to force re-run.
- `config.Config` is system-level only: `MAX_RETRIES`, `RETRY_BACKOFF`, `LOG_LEVEL`, `DATABASE_URL`, `LLM_BASE_URL`, `LLM_API_KEY`, `QDRANT_URL`, `QDRANT_API_KEY`.
- Pipeline inputs (repo URL, chunk params, etc.) are parsed inline by each `cmd/*/main.go`, not in config. Stage functions accept explicit parameters instead of `*config.Config` — see per-stage signatures.
- `go mod tidy` is only needed if adding external deps (currently none).
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

## Evaluation Harness

- `cmd/eval/main.go` — thin CLI that inserts a River `EvalArgs` job and polls until done
- `internal/workflow/eval_worker.go` — `EvalWorker` River worker that runs retrieval evaluation against an existing Qdrant collection, persists to `eval_runs`/`eval_queries` tables, and writes a JSON report
- `internal/eval/` — `ComputeAggregateMetrics` (HitRate, MRR, NDCG, Precision, Recall), `RetrievalEvaluator`, `EvalStore`, `PrintReport`/`WriteJSONReport`
- `testdata/eval/questions.json` — ground-truth dataset (fill with your questions)
- `internal/db/migrations/002_create_eval_tables.sql` — `eval_runs` + `eval_queries` tables
- `RunIndexing(ctx, args)` in `internal/workflow/index_worker.go` is shared between `IndexWorker` and `EvalWorker`
- Usage: `.\bin\eval.exe --index-tag idx-fixed-512 --query-strategy naive-search --dataset testdata/eval/questions.json`
- Run `.\bin\eval.exe --help` for all flags
