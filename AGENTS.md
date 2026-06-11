# AGENTS.md — GitLab Handbook RAG Pipeline

## Commands

```powershell
go build -o bin\workerd.exe .\cmd\workerd   # build worker daemon
go build -o bin\api.exe .\cmd\api           # build API server
go test ./...                               # all tests
make.cmd build                              # Docker-based build
make.cmd test                               # Docker-based test
docker compose up -d                        # full stack (pg+qdrant+api+workerd)
```

External dependencies via `go.mod`/`go.sum` — no `vendor/` dir. Run `go mod tidy` only when adding new deps.

## Package Map

| Package                  | Purpose                                                                   |
| ------------------------ | ------------------------------------------------------------------------- |
| `cmd/api/`               | HTTP server (chi router), all REST handlers + services                    |
| `cmd/workerd/`           | River worker daemon — registers PreprocessWorker, IndexWorker, EvalWorker |
| `internal/api/`          | Handlers, services, middleware, request/response types                    |
| `internal/workflow/`     | River workers + checkpointing + job polling                               |
| `internal/preprocessor/` | Hugo→clean markdown transforms (includes, shortcodes, HTML, refs)         |
| `internal/parser/`       | `.md` → `ElementReader` (strategy: `markdown`)                            |
| `internal/chunker/`      | Word-window splitting (strategy: `fixed`)                                 |
| `internal/embedder/`     | OpenAI-compatible embedder with batching + rate-limit retry               |
| `internal/generator/`    | OpenAI-compatible chat completion (sync + streaming)                      |
| `internal/store/`        | Qdrant gRPC backend (`VectorStore` interface)                             |
| `internal/retriever/`    | Embed + search strategies (strategy: `naive-search`)                      |
| `internal/eval/`         | Eval pipeline, metrics, judge, PgEvalStore CRUD                           |
| `internal/memory/`       | Per-conversation ring buffer for chat history                             |
| `internal/types/`        | Document, Chunk, Embedding, EvalQuestion, SearchResult, Element           |
| `internal/config/`       | Flag + env var loading, provider resolution                               |
| `internal/db/`           | PG pool, River client, SQL migrations                                     |

## Conventions & Patterns

- **Pipelines are River jobs** — each pipeline stage is a single durable job (preprocess, index, eval). The preprocess worker uses checkpointing via `river.JobUpdate` (stores `clone_done`/`preprocess_done` flags in job output). The eval worker checkpoints `questions_processed` for retry continuity.
- **No \*config.Config in workers** — stage functions accept explicit parameters from job args (e.g. `IndexArgs`, `EvalArgs`). System-level config (env vars) is read directly via `os.Getenv` inside workers.
- **Parser/chunker/retriever registry** — use `RegisterParser("name", fn)`, `RegisterChunker("name", fn)`, `RegisterRetriever("name", fn)` to add strategies. Currently registered: `parser: markdown`, `chunker: fixed`, `retriever: naive-search`.
- **Test helpers** — tests use `t.TempDir()` for temp dirs. Flag-based tests call `config.LoadWithEnv(flag.NewFlagSet(...), lookup, args)` — never use global `flag.Parse()`.
- **No journal caching** — the old `.journal/` gob cache was removed when River took over orchestration. The `workflow/store.go` file no longer exists either.
- **Clone target** — `artifacts/preprocessing/<tag>/repo/`, output is `artifacts/preprocessing/<tag>/output/`. Not tracked in git.

## Pipeline Architecture

```
Preprocess (1 job): Clone → Preprocess → Verify → cleaned markdown on disk
     ↓
Index (1 job): Parse → Chunk → Embed → Store (Qdrant) — files processed concurrently
     ↓
Eval (1 job): Batch embed queries → parallel eval (search + generate + judge, N workers)
```

The eval dataset is **JSONL** (one `EvalQuestion` per line), stored under `artifacts/preprocessing/<tag>/eval-dataset/`. Relevance judgments use `document_path` (e.g. `handbook/travel-policy.md`).

## Rate Limiting

**Two layers**, both active:

1. **Proactive** — token bucket via `EMBEDDER_RATE_LIMIT_RPM` (default 100) and `GENERATOR_RATE_LIMIT_RPM` (default 100) env vars. Each API call blocks on the bucket before executing.
2. **Reactive** — exponential backoff + jitter + `Retry-After` header respect on HTTP 429. Generator: up to 5 retries. Embedder: up to 5 retries.

The eval pipeline does **batch embedding** (reduces embed API calls from N to N/batchSize), then evaluates questions in parallel using configurable workers (default from `workers` field in `EvalRequest`). Each worker is a goroutine — concurrency is limited by the token bucket, not CPU.

## LLM Provider Resolution

`config.ResolveProviderConfig(provider)` maps provider name to env vars:

| Provider     | API Key                          | Base URL                                                                      |
| ------------ | -------------------------------- | ----------------------------------------------------------------------------- |
| `openai`     | `OPENAI_API_KEY` → `LLM_API_KEY` | `OPENAI_BASE_URL` → `LLM_BASE_URL` → `https://api.openai.com`                 |
| `gemini`     | `GEMINI_API_KEY`                 | `GEMINI_BASE_URL` → `https://generativelanguage.googleapis.com/v1beta/openai` |
| `openrouter` | `OPENROUTER_API_KEY`             | `OPENROUTER_BASE_URL` → `https://openrouter.ai/api/v1`                        |
| `lmstudio`   | _(none)_                         | `LMSTUDIO_BASE_URL` → `http://localhost:1234/v1`                              |

Factories: `embedder.New(provider, model, batchSize)` and `generator.New(provider, model)`.

## Embedding Model Dimension Mismatch

`EnsureCollection` skips if Qdrant collection exists — **no dimension validation**. Switching models may cause upsert failure. Fix: delete collection or use a different `tag` (collection name).

## Rebuild workerd after changes

River workers run in their own binary. After changing worker logic:

```powershell
go build -o bin\workerd.exe .\cmd\workerd
```

Then restart the process (or `docker compose restart workerd`).

## Testing

Unit tests **must not** interact with real infrastructure (Qdrant, Postgres, OpenAI, etc.) — not even locally. All external dependencies are behind Go interfaces (`VectorStore`, `Embedder`, `Generator`) and must be mocked in tests using testify mocks. Tests that genuinely need real infrastructure belong in a separate `_test.go` file with `t.Skip("requires X")` and are excluded from `go test ./...`.

Key coverage: preprocessor, chunker, embedder, generator, eval metrics, store, types, config, workflow, memory, retriever, db migrations.
