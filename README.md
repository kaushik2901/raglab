# GitLab Handbook RAG Pipeline

End-to-end pipeline that transforms GitLab's Hugo-based handbook (~4,500 pages) into a searchable RAG system with evaluation and streaming chat. Orchestrated by [River](https://riverqueue.com) on Postgres, with vectors stored in Qdrant and an OpenAI-compatible LLM backend.

<sup>**Stack:** Go · Postgres 16 · Qdrant · React (TypeScript, shadcn/ui, Recharts) · Docker</sup>

## Quick Start

```bash
docker compose up -d
```

Starts Postgres, Qdrant, the API server (`:8080`), and the workerd daemon. Then trigger the preprocessing pipeline:

```bash
curl -X POST http://localhost:8080/api/v1/workflows/preprocess \
  -H "Content-Type: application/json" \
  -d '{"repo_url":"https://gitlab.com/gitlab-com/content-sites/handbook.git","tag":"v1"}'
```

For local Go development:

```powershell
go build -o bin\api.exe .\cmd\api
go build -o bin\workerd.exe .\cmd\workerd
```

## Architecture

```mermaid
flowchart LR
    API["REST API (cmd/api)"] --> PG[("Postgres<br/>River + State")]
    PG --> PREW["Preprocess<br/>Clone → Clean → Verify"]
    PG --> INDW["Index<br/>Parse → Chunk → Embed → Store"]
    PG --> EVALW["Eval<br/>Batch Embed → Parallel Eval"]
    INDW --> QDRANT[("Qdrant<br/>gRPC")]
    PREW -.->|cleaned markdown<br/>on disk| INDW
    EVALW --> QDRANT
    API --> QDRANT
```

Two binaries: **`api.exe`** (HTTP server) and **`workerd.exe`** (River worker pool). The API inserts jobs into River; workers pick them up with at-least-once execution and retry. Postgres is the single source of truth for workflow state, eval results, and job tracking. All four LLM providers (OpenAI, Gemini, OpenRouter, LM Studio) speak the same OpenAI-compatible API format.

The React dashboard (`web/`) is a standalone Vite SPA with collapsible sidebar navigation.

![Dashboard](docs/images/001-Dashboard.png)

## Pipelines

### 1. Preprocess — `POST /api/v1/workflows/preprocess`

Shallow clone the handbook repo, transform Hugo into clean markdown, and write a verification report.

| Step | Description |
|------|-------------|
| Clone | `git clone --depth 1` or fast-forward if already cloned |
| Preprocess | Resolve `{{% include %}}`, strip shortcodes, clean HTML, resolve `{{< ref >}}` links |
| Verify | Write `_verification_report.json` (file count, stray artifacts, size distribution) |

Output lands in `artifacts/preprocessing/<tag>/output/`. Checkpoints (`clone_done`/`preprocess_done`) stored in River job metadata allow crash recovery from the last completed step.

### 2. Index — `POST /api/v1/workflows/index`

Reads preprocessed markdown, chunks documents, embeds via any OpenAI-compatible API, and upserts vectors into Qdrant.

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
  "index_concurrency": 5
}
```

Files are processed concurrently via `errgroup.SetLimit(concurrency)`. A probe query auto-detects vector dimensions, and `EnsureCollection` creates the Qdrant collection on first run. **Note:** if the collection already exists, dimensions are not validated — use a different `tag` when switching models.

### 3. Eval — `POST /api/v1/workflows/eval`

Measures retrieval and generation quality against a ground-truth dataset (JSONL, one question per line). Uses a two-phase strategy: batch-embed all queries, then evaluate in parallel with configurable workers.

Metrics stored in Postgres (`eval_runs`, `eval_queries`):

| Metric | Description |
|--------|-------------|
| HitRate@K | Proportion of questions with ≥1 relevant doc in top-K |
| MRR | Mean Reciprocal Rank of the first relevant result |
| NDCG@K | Binary and graded (0–3) NDCG |
| Precision@K / Recall@K | Standard retrieval metrics |
| AvgAnswerScore | LLM-as-judge correctness (0–1) |

Relevance judgments use `document_path` (e.g. `handbook/travel-policy.md`).

![Comparison](docs/images/002-Comparison.png)

### 4. Chat

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/chat/chat` | Non-streaming RAG answer with sources, tokens, latency |
| `POST /api/v1/chat/chat/stream` | SSE streaming (`retrieval` → `token`* → `done` events) |

Both accept `tag`, `query`, `top_k`, `temperature`, `max_tokens`, and an optional `conversation_id` for ring-buffer chat history (configurable via `CHAT_MEMORY_SIZE`, default 10 turns).

## API Reference

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/` | Embedded API test harness |
| `GET` | `/health` | Postgres + Qdrant health check |
| `GET` | `/api/v1/artifacts` | List preprocessed artifacts (`?type=&tag=`) |
| `GET` | `/api/v1/workflows` | List River jobs (`?kind=&state=&limit=&offset=`) |
| `GET` | `/api/v1/workflows/{id}` | Job status (available/running/completed/failed) |
| `POST` | `/api/v1/workflows/preprocess` | Start preprocess pipeline |
| `POST` | `/api/v1/workflows/index` | Start index pipeline |
| `POST` | `/api/v1/workflows/eval` | Start eval pipeline |
| `POST` | `/api/v1/chat/chat` | Synchronous RAG chat |
| `POST` | `/api/v1/chat/chat/stream` | Streaming RAG chat (SSE) |
| `GET` | `/api/v1/eval/runs` | List eval runs |
| `GET` | `/api/v1/eval/runs/{id}` | Eval run detail (paginated) |
| `GET` | `/api/v1/eval/runs/{id}/compare` | Compare up to 5 eval runs |
| `DELETE` | `/api/v1/eval/runs/{id}` | Delete eval run |

## Configuration

| Env Var | Default | Purpose |
|---------|---------|---------|
| `DATABASE_URL` | `postgres://rag:rag@localhost:5432/rag?sslmode=disable` | Postgres connection |
| `QDRANT_URL` | `http://localhost:6334` | Qdrant gRPC endpoint |
| `QDRANT_API_KEY` | — | Qdrant auth |
| `API_PORT` | `8080` | HTTP server port |
| `WORKER_CONCURRENCY` | `20` | River worker pool size |
| `EMBEDDER_RATE_LIMIT_RPM` | `100` | Proactive token bucket (embedder) |
| `GENERATOR_RATE_LIMIT_RPM` | `100` | Proactive token bucket (generator) |
| `CHAT_MEMORY_SIZE` | `10` | Ring buffer turns per conversation |
| `ARTIFACTS_DIR` | `artifacts` | Preprocessing output root |
| `LOG_LEVEL` | `info` | debug / info / warn / error |
| `MAX_RETRIES` | `3` | Stage max retry count |
| `RETRY_BACKOFF` | `5s` | Retry backoff duration |

### LLM Providers

| Provider | API Key | Base URL |
|----------|---------|----------|
| `openai` | `OPENAI_API_KEY` → `LLM_API_KEY` | `OPENAI_BASE_URL` → `LLM_BASE_URL` → `https://api.openai.com` |
| `gemini` | `GEMINI_API_KEY` | `GEMINI_BASE_URL` → `https://generativelanguage.googleapis.com/v1beta/openai` |
| `openrouter` | `OPENROUTER_API_KEY` | `OPENROUTER_BASE_URL` → `https://openrouter.ai/api/v1` |
| `lmstudio` | — | `LMSTUDIO_BASE_URL` → `http://localhost:1234/v1` |

### Rate Limiting

Two layers: **proactive** (token bucket) + **reactive** (exponential backoff with jitter, respects `Retry-After` on 429). Up to 5 retries for both embedder and generator.

## Project Structure

```
cmd/
  api/main.go           HTTP server (chi router)
  workerd/main.go       River worker daemon
internal/
  api/                  Handlers, services, middleware, types
  workflow/             River workers + checkpointing
  preprocessor/         Hugo → clean markdown transforms
  parser/               Markdown → element reader
  chunker/              Fixed-size word-window chunker
  embedder/             OpenAI-compatible embedder (batch + retry)
  generator/            OpenAI-compatible chat (sync + streaming)
  store/                Qdrant gRPC backend
  retriever/            Retrieval strategies
  eval/                 Pipeline, metrics, judge, Postgres store
  memory/               Per-conversation ring buffer
  types/                Document, Chunk, Embedding, SearchResult, etc.
  config/               Flag + env loading, provider resolution
  db/                   PG pool, River client, migrations
web/
  src/                  React dashboard (Vite + shadcn/ui + Recharts)
```

Parser, chunker, and retriever use a registry pattern: `RegisterParser("name", fn)`, `RegisterChunker("name", fn)`, `RegisterRetriever("name", fn)`.

## Testing

```bash
go test ./...
```

Unit tests use testify mocks and require no infrastructure. Integration tests that need Postgres or Qdrant use `t.Skip(...)` and are excluded from default runs.

## License

MIT
