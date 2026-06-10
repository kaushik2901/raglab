# GitLab Handbook RAG Pipeline

Durable, multi-stage pipeline that converts GitLab's Hugo-based handbook (~4,500 markdown files) into a searchable RAG system. Preprocessing → vector indexing → evaluation → chat, orchestrated via [River](https://riverqueue.com) (Postgres-backed job queue with at-least-once execution and retry).

## Quick Start

```bash
docker compose up -d
```

This starts **Postgres 16**, **Qdrant**, the **API server** (`:8080`), and the **workerd** daemon. For local development, set `DATABASE_URL` and `QDRANT_URL` to point at the containers and run binaries directly:

```bash
go build -o bin\workerd.exe .\cmd\workerd
go build -o bin\api.exe .\cmd\api
```

Or use `make.cmd build`.

```bash
curl -X POST http://localhost:8080/api/v1/workflows/preprocess \
  -H "Content-Type: application/json" \
  -d '{"repo_url":"https://gitlab.com/gitlab-com/content-hash/handbook.git","tag":"v1"}'
```

## Architecture

```mermaid
flowchart LR
    API["REST API (cmd/api)"] --> PG[("Postgres<br/>River + State")]
    PG --> PREW["Clone → Preprocess → Verify"]
    PG --> INDW["Parse → Chunk → Embed → Store"]
    PG --> EVALW["Batch Embed → Parallel Eval"]
    INDW --> QDRANT[("Qdrant<br/>gRPC")]
    PREW -.->|cleaned markdown<br/>on disk| INDW
    EVALW --> QDRANT
    API --> QDRANT
```

Two processes: **`api.exe`** (HTTP server) and **`workerd.exe`** (River worker pool).  
The API inserts River jobs; workers execute them. Postgres is the single source of truth for workflow state, eval results, and River's internal job tracking.

## Pipelines

### Preprocessing (`POST /api/v1/workflows/preprocess`)

Clones the handbook repo (shallow, `--depth 1`) and transforms Hugo markdown into clean LLM-friendly markdown:

| Step           | What it does                                                                                         |
| -------------- | ---------------------------------------------------------------------------------------------------- |
| **Clone**      | `git clone --depth 1`; if exists: fetch + checkout main + pull --ff-only                             |
| **Preprocess** | Resolve `{{% include %}}`, strip Hugo shortcodes, clean HTML, resolve `{{< ref >}}`/`{{< relref >}}` |
| **Verify**     | Writes `_verification_report.json` (file count, dir structure, stray shortcodes/HTML, content size)  |

Output: `artifacts/preprocessing/<tag>/output/` — 10 parallel goroutines per file batch.  
Checkpointing: resumes from last completed step on crash (River job output stores `clone_done`/`preprocess_done` flags).

### Indexing (`POST /api/v1/workflows/index`)

Reads the preprocessed output, chunks documents, embeds via any OpenAI-compatible API, and stores vectors in Qdrant.

```json
{
  "input_tag": "v1",
  "tag": "v1-index",
  "parser_strategy": "markdown",
  "chunk_strategy": "fixed",
  "chunk_size": 500,
  "chunk_overlap": 50,
  "embedding_provider": "openai",
  "embedding_model": "text-embedding-3-small",
  "batch_size": 20,
  "index_concurrency": 5,
  "doc_timeout": "5m"
}
```

Files processed concurrently via `errgroup.SetLimit(concurrency)`. Each file: parser reads elements → fixed-window word chunker → batch embed → upsert to Qdrant. Probe query auto-detects vector dimension and `EnsureCollection` creates the Qdrant collection on first run.

### Evaluation (`POST /api/v1/workflows/eval`)

Measures retrieval + generation quality against ground-truth datasets (JSONL format, one `EvalQuestion` per line). Uses a **batch-embed-then-parallel-eval** strategy:

1. **Batch embed** all query texts (reduces API calls from N to N/batchSize)
2. **Parallel eval**: N workers each run search → generate → judge per question

Metrics stored in Postgres (`eval_runs`, `eval_queries` tables).

| Metric                 | Description                                           |
| ---------------------- | ----------------------------------------------------- |
| HitRate@K              | Proportion of questions with ≥1 relevant doc in top-K |
| MRR                    | Mean Reciprocal Rank of first relevant result         |
| NDCG@K                 | Binary relevance NDCG                                 |
| NDCGGraded@K           | Graded (0–3) relevance NDCG                           |
| Precision@K / Recall@K | Standard retrieval metrics                            |
| AvgAnswerScore         | LLM-as-judge correctness (0–1)                        |

Relevance judgments use `document_path` (e.g. `handbook/travel-policy.md`).

### Chat API

| Endpoint                        | Description                                |
| ------------------------------- | ------------------------------------------ |
| `POST /api/v1/chat/chat`        | Non-streaming RAG answer                   |
| `POST /api/v1/chat/chat/stream` | SSE streaming (`event: token/data: {...}`) |

Both take `tag`, `query`, `top_k`, `temperature`, `max_tokens`, `embedding_*`, `llm_*`, and optional `conversation_id` for ring-buffer history. The streaming version also emits `event: retrieval` (sources) and `event: done` (final usage).

## API Reference

| Method | Path                                            | Purpose                                         |
| ------ | ----------------------------------------------- | ----------------------------------------------- |
| `GET`  | `/health`                                       | Postgres + Qdrant health check                  |
| `GET`  | `/`                                             | Landing page (embedded HTML)                    |
| `GET`  | `/api/v1/artifacts?type=&tag=`                  | List preprocessed artifacts                     |
| `POST` | `/api/v1/workflows/preprocess`                  | Start preprocess job                            |
| `POST` | `/api/v1/workflows/index`                       | Start index job                                 |
| `POST` | `/api/v1/workflows/eval`                        | Start eval job                                  |
| `GET`  | `/api/v1/workflows/{id}`                        | Job status (available/running/completed/failed) |
| `POST` | `/api/v1/chat/chat`                             | RAG chat                                        |
| `POST` | `/api/v1/chat/chat/stream`                      | Streaming RAG chat                              |
| `GET`  | `/api/v1/eval/runs`                             | List eval runs                                  |
| `GET`  | `/api/v1/eval/runs/{id}`                        | Eval run detail (paginated)                     |
| `GET`  | `/api/v1/eval/runs/{id}/compare?compare_to=...` | Compare eval runs                               |

## Configuration

System-level config via environment variables (also overridable via CLI flags for `--max-retries`, `--retry-backoff`, `--log-level`):

| Env Var                    | Default                                                 | Purpose                            |
| -------------------------- | ------------------------------------------------------- | ---------------------------------- |
| `DATABASE_URL`             | `postgres://rag:rag@localhost:5432/rag?sslmode=disable` | Postgres connection                |
| `QDRANT_URL`               | `http://localhost:6334`                                 | Qdrant gRPC endpoint               |
| `QDRANT_API_KEY`           | `""`                                                    | Qdrant auth (optional)             |
| `MAX_RETRIES`              | `3`                                                     | Stage max retry count              |
| `RETRY_BACKOFF`            | `5s`                                                    | Retry backoff duration             |
| `LOG_LEVEL`                | `info`                                                  | Log level (debug/info/warn/error)  |
| `WORKER_CONCURRENCY`       | `20`                                                    | River worker pool size             |
| `API_PORT`                 | `8080`                                                  | HTTP server port                   |
| `API_REQUEST_TIMEOUT`      | `60s`                                                   | Per-request timeout                |
| `CHAT_MEMORY_SIZE`         | `10`                                                    | Ring buffer turns per conversation |
| `ARTIFACTS_DIR`            | `artifacts`                                             | Preprocessing output root          |
| `EMBEDDER_RATE_LIMIT_RPM`  | `100`                                                   | Proactive token bucket (embedder)  |
| `GENERATOR_RATE_LIMIT_RPM` | `100`                                                   | Proactive token bucket (generator) |

### LLM Providers

All providers use OpenAI-compatible API format internally. The `--llm-provider` flag / provider name selects which env vars to read:

| Provider     | API Key                          | Base URL                                                                      |
| ------------ | -------------------------------- | ----------------------------------------------------------------------------- |
| `openai`     | `OPENAI_API_KEY` → `LLM_API_KEY` | `OPENAI_BASE_URL` → `LLM_BASE_URL` → `https://api.openai.com`                 |
| `gemini`     | `GEMINI_API_KEY`                 | `GEMINI_BASE_URL` → `https://generativelanguage.googleapis.com/v1beta/openai` |
| `openrouter` | `OPENROUTER_API_KEY`             | `OPENROUTER_BASE_URL` → `https://openrouter.ai/api/v1`                        |
| `lmstudio`   | _(none)_                         | `LMSTUDIO_BASE_URL` → `http://localhost:1234/v1`                              |

### Rate Limiting

Two layers: **proactive** (token bucket via `EMBEDDER_RATE_LIMIT_RPM`/`GENERATOR_RATE_LIMIT_RPM`) + **reactive** (exponential backoff with jitter + `Retry-After` header respect on 429). Generator: up to 5 retries. Embedder: up to 5 retries.

## Project Structure

```
cmd/
  api/main.go           — HTTP API server (chi router)
  workerd/main.go       — River worker daemon (preprocess + index + eval workers)

internal/
  api/                  — REST handlers, services, middleware, types
  workflow/             — River workers + checkpointing
  preprocessor/         — Hugo→clean markdown (includes, shortcodes, HTML, refs)
  parser/               — .md → ElementReader (strategy: markdown)
  chunker/              — Word-window splitting (strategy: fixed)
  embedder/             — OpenAI-compatible embedder with batching + rate-limit retry
  generator/            — OpenAI-compatible generator (chat + streaming)
  store/                — Qdrant gRPC backend (VectorStore interface)
  retriever/            — Retrieval strategies (strategy: naive-search)
  memory/               — Per-conversation ring buffer
  eval/                 — Pipeline, metrics, judge, store (eval_runs/query tables)
  types/                — Document, Chunk, Embedding, EvalQuestion, etc.
  config/               — Flag + env var loading, provider resolution
  db/                   — PG pool, River client, migrations

docker-compose.yml      — Postgres 16 + Qdrant (+ optional api/workerd containers)
```

### Extensibility

Parser, chunker, and retriever use a registry pattern with `Register*` functions:

- `parser.RegisterParser("name", fn)` — add a new parser strategy
- `chunker.RegisterChunker("name", fn)` — add a new chunker strategy
- `retriever.RegisterRetriever("name", fn)` — add a new retrieval strategy

## Embedding Dimension Mismatch

`EnsureCollection` returns immediately if the Qdrant collection exists — it **does not** validate vector dimensions. Switching embedding models may cause upsert failures. Workaround: use a unique `tag` (collection name) per model, or delete the collection manually.

## Testing

```bash
go test ./...
```

Tests requiring Postgres or Qdrant use `t.Skip(...)` and are excluded by default. Key areas: preprocessor, chunker, embedder, generator, eval metrics, store, types, config, workflow, memory, retriever, db migrations.

## License

MIT
