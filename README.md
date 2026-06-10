# GitLab Handbook RAG Pipeline

Preprocessing + indexing + evaluation pipeline for GitLab's public handbook (~4,500 markdown files). Converts Hugo-based documentation into clean, LLM/RAG-friendly markdown, indexes it into a Qdrant vector store, and evaluates retrieval quality.

Uses **River** (`github.com/riverqueue/river`) as the job engine — a lightweight Go queue backed by Postgres for durable at-least-once execution, retries with backoff, and concurrency control.

## Architecture

```mermaid
flowchart LR
    subgraph API["API Layer"]
        API_SERVER[cmd/api<br/>REST API]
    end

    subgraph PG["Postgres (River + State)"]
        WF[workflows<br/>workflow_steps]
        ER[eval_runs<br/>eval_queries]
        RJ[river_job]
    end

    subgraph WORKER["workerd Process (River Workers)"]
        PREW[Clone → Preprocess → Verify<br/>1 worker]
        INDW[Parse → Chunk → Embed → Store<br/>1 worker, parallel files]
        EVALW[Retrieve → Generate → Judge<br/>1 worker, sequential questions]
    end

    subgraph STORE["Vector Store"]
        QDRANT[(Qdrant<br/>gRPC)]
    end

    API_SERVER --> PG
    PG --> PREW
    PG --> INDW
    PG --> EVALW
    INDW --> QDRANT
    PREW -.->|cleaned markdown<br/>on disk| INDW
    EVALW --> QDRANT
    API_SERVER --> QDRANT
```

Workflows are triggered via the REST API and executed by River workers in the `workerd` process. The preprocessing pipeline writes cleaned markdown to disk; the indexing pipeline reads it back and stores vectors in Qdrant; the eval pipeline measures retrieval quality.

### Pipeline Stage Flow

```mermaid
flowchart TD
    subgraph PREPROC["Preprocessing Pipeline"]
        C[Clone] --> P[Preprocess]
        P --> V[Verify]
        V --> OUT[(Cleaned Markdown<br/>on disk)]
    end

    subgraph INDEX["Indexing Pipeline"]
        PA[Parse .md files] --> CH[Fixed-Window Chunk]
        CH --> EM[Embed<br/>OpenAI-compatible API]
        EM --> ST[Store in Qdrant]
    end

    subgraph EVAL["Evaluation Pipeline"]
        BATCH[Phase 1: Batch Embed<br/>all queries] --> SEARCH[Phase 2: Sequential<br/>Qdrant Search]
        SEARCH --> GEN[Phase 3: Sequential<br/>LLM Generate]
        GEN --> JUDGE[Phase 4: Sequential<br/>Judge Answer]
    end

    OUT -.->|reads from| PA
```

### Evaluation 4-Phase Detail

```mermaid
sequenceDiagram
    participant API as API Server
    participant WK as workerd (eval worker)
    participant Q as Qdrant
    participant LLM as LLM API

    API->>WK: Insert eval River job
    WK->>WK: Batch embed all N queries
    loop For each question (1..N)
        WK->>Q: Search (query vector)
        Q-->>WK: top-K results
        WK->>LLM: Generate answer (context + question)
        LLM-->>WK: answer + tokens
        WK->>LLM: Judge (question + expected + answer)
        LLM-->>WK: correctness score (0-1)
    end
    WK->>WK: Compute aggregate metrics
```

## Prerequisites

```bash
docker compose up -d
```

## Preprocessing Pipeline

Transforms Hugo markdown into clean markdown suitable for LLM ingestion.

### Stages

1. **clone** — clones the handbook repo (or pulls latest if already present)
2. **preprocess** — transforms each markdown file (parallelized, 10 goroutines):
   - Resolves `{{% include "path" %}}` directives (recursive with cycle detection)
   - Strips Hugo shortcodes (details, alert, panel, youtube, etc.)
   - Cleans raw HTML (style, script, iframe, img, a, table, div, etc.)
   - Resolves `{{< ref >}}` / `{{< relref >}}` to markdown links
3. **verify** — validates output quality (file count, directory structure, no stray shortcodes/HTML, minimum content size, total size sanity)

Artifacts are stored at `artifacts/preprocessing/<tag>/repo/` and `artifacts/preprocessing/<tag>/output/`.

The preprocess worker runs clone → preprocess → verify as a single River job with checkpointing (resumes from the last completed step on crash). Trigger via `POST /api/v1/workflows/preprocess`.

## Indexing Pipeline

Builds on the preprocessing output. Reads cleaned markdown, chunks documents, generates embeddings, and stores vectors in Qdrant. **Files are processed concurrently** (configurable via `index_concurrency` in the request payload).

### Stages

1. **parse** — reads all `.md` files from the preprocessed output directory
2. **chunk** — splits documents into fixed-size word windows with configurable overlap
3. **embed** — sends chunks to any OpenAI-compatible embedding API (OpenAI, OpenRouter, LM Studio, Ollama)
4. **store** — upserts document chunks with embeddings into Qdrant via gRPC

Trigger via `POST /api/v1/workflows/index` with all parameters in the request payload.

## Evaluation Pipeline

Measures retrieval and generation quality against ground-truth datasets. Uses a **4-phase pipeline**: batch embed all queries first, then sequential search → generate → judge for each question.

Relevance judgments use `document_path` (the relative path within the preprocessed output, e.g. `handbook/travel-policy.md`) instead of a document ID.

### Metrics

- **HitRate@K** — proportion of questions with at least one relevant document in top-K
- **MRR** — Mean Reciprocal Rank of the first relevant result
- **NDCG@K** — Normalized Discounted Cumulative Gain (binary)
- **NDCGGraded@K** — NDCG with graded relevance (0–3 scale, supports partial relevance)
- **AvgAnswerScore** — LLM-as-judge correctness score (0–1) comparing generated answer against `expected_answer`
- **Precision@K** — proportion of retrieved documents that are relevant
- **Recall@K** — proportion of relevant documents that are retrieved

Trigger via `POST /api/v1/workflows/eval` with all parameters in the request payload. Dataset files follow the `EvalDataset` format (a `meta` object + `questions` array) with `document_path` in relevance judgments.

## Chat API

Semantic search with RAG-based answer generation is available via `POST /api/v1/chat/chat` (non-streaming) and `POST /api/v1/chat/chat/stream` (SSE streaming). All parameters are explicit in the request payload.

## Environment Variables

Secrets and connection strings are read from environment variables:

| Env Var               | Default                                                      | Description                            |
| --------------------- | ------------------------------------------------------------ | -------------------------------------- |
| `LLM_API_KEY`         | `""`                                                         | LLM API key (overridable per-provider) |
| `LLM_BASE_URL`        | `https://api.openai.com`                                     | LLM API base URL (overridable per-provider) |
| `OPENAI_API_KEY`      | (falls back to `LLM_API_KEY`)                                | OpenAI API key                        |
| `OPENAI_BASE_URL`     | (falls back to `LLM_BASE_URL`) → `https://api.openai.com`    | OpenAI API base URL                   |
| `GEMINI_API_KEY`      | `""`                                                         | Google Gemini API key                 |
| `GEMINI_BASE_URL`     | `https://generativelanguage.googleapis.com/v1beta/openai`    | Gemini OpenAI-compat endpoint         |
| `OPENROUTER_API_KEY`  | `""`                                                         | OpenRouter API key                    |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1`                               | OpenRouter API base URL               |
| `LMSTUDIO_BASE_URL`   | `http://localhost:1234/v1`                                   | LM Studio base URL (no key)           |
| `WORKER_CONCURRENCY`  | `20`                                                         | River worker pool concurrency         |
| `QDRANT_URL`          | `http://localhost:6334`                                      | Qdrant gRPC endpoint                  |
| `QDRANT_API_KEY`      | `""`                                                         | Qdrant API key (optional)             |
| `DATABASE_URL`        | `postgres://rag:rag@localhost:5432/rag?sslmode=disable`      | Postgres connection string            |
| `MAX_RETRIES`         | `3`                                                          | Maximum retry count for stages        |
| `RETRY_BACKOFF`       | `5s`                                                         | Retry backoff duration                |
| `LOG_LEVEL`           | `info`                                                       | Log level                              |

### Provider Quick Reference

| Provider     | Env API Key                      | Env Base URL                       | Default Base URL                                          |
| ------------ | -------------------------------- | ---------------------------------- | --------------------------------------------------------- |
| `openai`     | `OPENAI_API_KEY` → `LLM_API_KEY` | `OPENAI_BASE_URL` → `LLM_BASE_URL` | `https://api.openai.com`                                  |
| `gemini`     | `GEMINI_API_KEY`                 | `GEMINI_BASE_URL`                  | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `openrouter` | `OPENROUTER_API_KEY`             | `OPENROUTER_BASE_URL`              | `https://openrouter.ai/api/v1`                            |
| `lmstudio`   | _(none)_                         | `LMSTUDIO_BASE_URL`                | `http://localhost:1234/v1`                                |

## Retry & Recovery

Each pipeline stage is executed as a River job, providing built-in:

- **Automatic retries** (configurable via `MAX_RETRIES`)
- **Exponential backoff** with jitter
- **At-least-once execution** — workers are retried on failure
- **Durable state** — workflow and step progress is persisted in Postgres

The `workerd` process must be running to execute jobs. If it crashes, pending jobs survive and are picked up on restart.

## Concurrency

Key concurrent processing in the pipeline:

| Component                    |       Concurrency        | Mechanism                          |
| ---------------------------- | :----------------------: | ---------------------------------- |
| Preprocessor file processing |      10 goroutines       | `errgroup.SetLimit(10)`            |
| Indexer file processing      | Configurable (default 5) | `errgroup.SetLimit(concurrency)`   |
| Eval question phases         |        Sequential        | Batch embed → sequential gen/judge |
| River worker pool            | Configurable (default 20)| `WORKER_CONCURRENCY` env var       |

The indexer uses `golang.org/x/sync/errgroup` with a bounded semaphore to process files in parallel. The eval pipeline is sequential by design — the rate limit (API's 429 + `Retry-After`) is the bottleneck, not CPU, so concurrency can't improve throughput.

## Project Structure

```
cmd/
  api/main.go            — HTTP API server
  workerd/main.go        — River worker daemon (all workers)

internal/
  config/config.go       — Configuration (flags + env vars)
  db/
    db.go                — PG connection pool
    migrate.go           — Schema migrations + River auto-migrate
    migrations/          — SQL migration files
  types/
    document.go          — Document type
    indexing.go          — Chunk, Embedding, DocumentChunk types
    workflow.go          — Workflow, WorkflowStep types
    eval.go              — EvalQuestion, RetrievalResult, AggregateMetrics, EvalReport
    query.go             — SearchResult type
  workflow/
    store.go             — Workflow/step CRUD + runStep helper
    poll.go              — PollUntilDone helper
    preprocess_worker.go — Preprocess worker (clone repo → preprocess → verify)
    index_worker.go      — Index worker (parse + chunk + embed + store)
    eval_worker.go       — Eval worker (retrieve + generate + metrics)
  preprocessor/
    includes.go          — {{% include %}} resolver
    shortcodes.go        — Shortcode stripper with rules engine
    html.go              — HTML cleaning
    refs.go              — {{< ref >}} / {{< relref >}} resolver
    preprocessor.go      — Orchestrator (applies all transforms)
  parser/parser.go       — Reads cleaned .md files into Documents
  chunker/
    chunker.go           — Chunker interface
    fixed.go             — Fixed-size word-window chunking
  embedder/
    embedder.go          — Embedder interface + factory (multi-provider)
    openai.go            — OpenAI-compatible HTTP embedder (with retry)
  store/
    store.go             — VectorStore interface
    qdrant.go            — Qdrant gRPC backend
  retriever/
    retriever.go         — Retrieval strategies (naive-search)
  memory/
    memory.go            — Per-conversation ring buffer for chat history
  generator/
    generator.go         — Generator interface + factory + OpenAI-compatible implementation
  eval/
    pipeline.go          — Phase-based evaluation (batch embed → sequential gen/judge)
    retrieval.go         — Legacy RetrievalEvaluator (kept for tests)
    metrics.go           — HitRate, MRR, NDCG, Precision, Recall computation
    report.go            — PrintReport + WriteJSONReport
    store.go             — EvalStore CRUD (eval_runs, eval_queries tables)

docs/
  river-implementation-plan.md
  project-vision-and-roadmap.md

artifacts/preprocessing/<tag>/eval-dataset/ — Ground-truth dataset files (.json, one workflow per file)
docker-compose.yml             — Postgres 16 + Qdrant
```

## Testing

```bash
go test ./...
```

Tests requiring a Qdrant or Postgres server use `t.Skip(...)` and are excluded from `go test ./...` by default.

Key unit test coverage areas:

| Package         | What's Tested                                                                                                                             |
| --------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `preprocessor/` | Shortcodes, includes, HTML, refs, file processing, concurrency defaults                                                                   |
| `chunker/`      | Word-window splitting, overlap clamping, empty/short docs, chunk IDs                                                                      |
| `embedder/`     | Batching, rate-limit retry, API errors, model fallback, headers                                                                           |
| `retriever/`    | Constructor, retrieve flow, embed/search error propagation                                                                                |
| `memory/`       | Ring buffer add/get/eviction/concurrent safety                                                                                            |
| `generator/`    | Generate with mock HTTP, API errors, URL normalization, 429 retry with Retry-After                                                        |
| `eval/`         | All metric computations, pipeline (embed→search→generate→judge), RetrievalEvaluator, WriteJSONReport, gradeForPath, idealGradedRelevances |
| `store/`        | Point conversion, payload encoding, chunk ID hashing, distance parsing                                                                    |
| `types/`        | All struct creation, zero values, round-trip serialization                                                                                |
| `config/`       | Validation, env overrides, ResolveTag                                                                                                     |
| `workflow/`     | Store CRUD, status transitions, state merging, Kind() methods, preprocess/index/eval workers                              |
| `db/`           | Migration version parsing                                                                                                                 |

## License

MIT
