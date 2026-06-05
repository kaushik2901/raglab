# GitLab Handbook RAG Pipeline

Preprocessing + indexing + evaluation pipeline for GitLab's public handbook (~4,500 markdown files). Converts Hugo-based documentation into clean, LLM/RAG-friendly markdown, indexes it into a Qdrant vector store, and evaluates retrieval quality.

Uses **River** (`github.com/riverqueue/river`) as the job engine — a lightweight Go queue backed by Postgres for durable at-least-once execution, retries with backoff, and concurrency control.

## Architecture

```
┌─────────────────────┐     ┌──────────────────────┐     ┌──────────────────────┐
│  cmd/preprocess     │     │  cmd/index           │     │  cmd/eval            │
│  (River job insert) │     │  (River job insert)  │     │  (River job insert)  │
└─────────┬───────────┘     └──────────┬───────────┘     └──────────┬───────────┘
          │                            │                            │
          ▼                            ▼                            ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Postgres (River + State)                          │
│  workflows │ workflow_steps │ eval_runs │ eval_queries │ river_job │ ...    │
└─────────────────────────────────────────────────────────────────────────────┘
          │                            │                            │
          ▼                            ▼                            ▼
┌──────────────────────┐    ┌──────────────────────┐     ┌──────────────────────┐
│  cmd/workerd         │    │  cmd/workerd         │     │  cmd/workerd         │
│  Clone → Preprocess  │    │  Parse → Chunk →     │     │  Retrieve → Generate │
│  → Verify            │    │  Embed → Store       │     │  → Eval Metrics      │
│  (3 workers)         │    │  (1 worker, parallel)│     │  (1 worker, parallel)│
└──────────────────────┘    └──────────────────────┘     └──────────────────────┘
                                                                    │
          ┌─────────────────────────────────────────────────────────┘
          ▼
┌──────────────────────┐
│  cmd/query           │
│  (synchronous CLI)   │
│  Embed → Search →    │
│  Generate            │
└──────────────────────┘
```

Three pipelines share a single `workerd` process. The preprocessing pipeline writes cleaned markdown to disk; the indexing pipeline reads it back and stores vectors in Qdrant; the eval pipeline measures retrieval quality. The query CLI runs synchronously (not a River workflow).

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

### Usage

```bash
go build -o bin\preprocess.exe .\cmd\preprocess
go build -o bin\workerd.exe .\cmd\workerd
.\bin\workerd.exe

.\bin\preprocess.exe
```

Artifacts are stored at `artifacts/preprocessing/<tag>/repo/` and `artifacts/preprocessing/<tag>/output/`.

### CLI Flags

| Flag             | Env Var        | Default                                                | Description                                      |
| ---------------- | -------------- | ------------------------------------------------------ | ------------------------------------------------ |
| `--repo-url`     | `REPO_URL`     | `https://gitlab.com/gitlab-com/content-sites/handbook` | Handbook repository URL                          |
| `--tag`          | `TAG`          | `pre-<timestamp>`                                      | Workflow tag                                     |
| `--include-dirs` | `INCLUDE_DIRS` | `""`                                                   | Comma-separated subdirs to process (empty = all) |

## Indexing Pipeline

Builds on the preprocessing output. Reads cleaned markdown, chunks documents, generates embeddings, and stores vectors in Qdrant. **Files are processed concurrently** (configurable via `--index-concurrency`).

### Stages

1. **parse** — reads all `.md` files from the preprocessed output directory
2. **chunk** — splits documents into fixed-size word windows with configurable overlap
3. **embed** — sends chunks to any OpenAI-compatible embedding API (OpenAI, OpenRouter, LM Studio, Ollama)
4. **store** — upserts document chunks with embeddings into Qdrant via gRPC

### Usage

```bash
go build -o bin\index.exe .\cmd\index
.\bin\workerd.exe

.\bin\index.exe --input-tag pre-20260603-141651
```

### CLI Flags

| Flag                   | Env Var              | Default                    | Description                                                 |
| ---------------------- | -------------------- | -------------------------- | ----------------------------------------------------------- |
| `--input-tag`          | `INPUT_TAG`          | —                          | **Required.** Preprocessed output tag to index              |
| `--tag`                | `TAG`                | `idx-<timestamp>`          | Workflow tag                                                |
| `--llm-provider`       | `LLM_PROVIDER`       | `openai`                   | LLM provider (`openai`, `gemini`, `openrouter`, `lmstudio`) |
| `--embedding-provider` | `EMBEDDING_PROVIDER` | (same as `--llm-provider`) | Embedding provider                                          |
| `--chunk-strategy`     | `CHUNK_STRATEGY`     | `fixed`                    | Chunking strategy (fixed only)                              |
| `--chunk-size`         | `CHUNK_SIZE`         | `512`                      | Target token count per chunk                                |
| `--chunk-overlap`      | `CHUNK_OVERLAP`      | `64`                       | Token overlap between chunks                                |
| `--embedding-model`    | `EMBEDDING_MODEL`    | `text-embedding-3-small`   | Embedding model name                                        |
| `--batch-size`         | `BATCH_SIZE`         | `20`                       | Embedding API batch size                                    |
| `--index-concurrency`  | `INDEX_CONCURRENCY`  | `5`                        | Number of files to index concurrently                       |

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

### Usage

```bash
go build -o bin\eval.exe .\cmd\eval
.\bin\workerd.exe

.\bin\eval.exe --index-tag idx-fixed-512 --query-strategy naive-search --dataset-dir artifacts/preprocessing/pre-20260603-141651/eval-dataset
```

### CLI Flags

| Flag                   | Env Var              | Default                    | Description                                                         |
| ---------------------- | -------------------- | -------------------------- | ------------------------------------------------------------------- |
| `--index-tag`          | —                    | —                          | **Required.** Existing Qdrant collection name to evaluate           |
| `--query-strategy`     | —                    | —                          | **Required.** Query strategy (`naive-search`)                       |
| `--dataset-dir`        | —                    | —                          | **Required.** Directory containing `.json` evaluation dataset files |
| `--top-k`              | —                    | `5`                        | Top-K retrieval                                                     |
| `--llm-provider`       | `LLM_PROVIDER`       | `openai`                   | LLM provider (`openai`, `gemini`, `openrouter`, `lmstudio`)         |
| `--embedding-provider` | `EMBEDDING_PROVIDER` | (same as `--llm-provider`) | Embedding provider for query embedding                              |
| `--embedding-model`    | `EMBEDDING_MODEL`    | `text-embedding-3-small`   | Embedding model for query vectorization                             |
| `--judge-provider`     | `JUDGE_PROVIDER`     | (same as `--llm-provider`) | Judge provider for answer scoring                                   |
| `--llm-model`          | `LLM_MODEL`          | `gpt-4o-mini`              | LLM model for answer generation                                     |
| `--judge-model`        | `JUDGE_MODEL`        | (same as `--llm-model`)    | LLM model for answer correctness scoring                            |
| `--tag`                | —                    | `eval-<timestamp>`         | Eval run tag prefix                                                 |

Each `.json` file in the dataset directory is evaluated in its own River workflow (all submitted concurrently). Dataset files follow the `EvalDataset` format (a `meta` object + `questions` array) with `document_path` in relevance judgments.

## Query CLI

Interactive or one-shot semantic search against an indexed collection. Runs synchronously — no River workflow.

```bash
go build -o bin\query.exe .\cmd\query
.\bin\query.exe --tag idx-fixed-512 --query "How do I set up SSH?"
```

### CLI Flags

| Flag                   | Env Var              | Default                    | Description                                                 |
| ---------------------- | -------------------- | -------------------------- | ----------------------------------------------------------- |
| `--tag`                | —                    | —                          | **Required.** Qdrant collection name                        |
| `--query`              | —                    | —                          | One-shot query (omit for interactive mode)                  |
| `--top-k`              | —                    | `5`                        | Number of results to retrieve                               |
| `--query-strategy`     | —                    | `naive-search`             | Retrieval strategy                                          |
| `--llm-provider`       | `LLM_PROVIDER`       | `openai`                   | LLM provider (`openai`, `gemini`, `openrouter`, `lmstudio`) |
| `--embedding-provider` | `EMBEDDING_PROVIDER` | (same as `--llm-provider`) | Embedding provider                                          |
| `--embedding-model`    | `EMBEDDING_MODEL`    | `text-embedding-3-small`   | Embedding model for query                                   |
| `--llm-model`          | `LLM_MODEL`          | `gpt-4o-mini`              | LLM model for answer generation                             |
| `--temperature`        | —                    | `0.3`                      | LLM temperature                                             |
| `--max-tokens`         | —                    | `1024`                     | Max answer tokens                                           |
| `--conversation-id`    | —                    | `""`                       | Conversation ID for multi-turn memory                       |

## Environment Variables

Secrets and connection strings are read from environment variables (not CLI flags). The `--llm-provider` flag selects which set of vars to use:

| Env Var               | Default                                                      | Description                    |
| --------------------- | ------------------------------------------------------------ | ------------------------------ |
| `LLM_PROVIDER`        | `openai`                                                     | LLM provider name              |
| `OPENAI_API_KEY`      | (falls back to `LLM_API_KEY`)                                | OpenAI API key                 |
| `OPENAI_BASE_URL`     | (falls back to `LLM_BASE_URL`) → `https://api.openai.com/v1` | OpenAI API base URL            |
| `GEMINI_API_KEY`      | `""`                                                         | Google Gemini API key          |
| `GEMINI_BASE_URL`     | `https://generativelanguage.googleapis.com/v1beta/openai`    | Gemini OpenAI-compat endpoint  |
| `OPENROUTER_API_KEY`  | `""`                                                         | OpenRouter API key             |
| `OPENROUTER_BASE_URL` | `https://openrouter.ai/api/v1`                               | OpenRouter API base URL        |
| `LMSTUDIO_BASE_URL`   | `http://localhost:1234/v1`                                   | LM Studio base URL (no key)    |
| `QDRANT_URL`          | `http://localhost:6334`                                      | Qdrant gRPC endpoint           |
| `QDRANT_API_KEY`      | `""`                                                         | Qdrant API key (optional)      |
| `DATABASE_URL`        | `postgres://rag:rag@localhost:5432/rag?sslmode=disable`      | Postgres connection string     |
| `MAX_RETRIES`         | `3`                                                          | Maximum retry count for stages |
| `RETRY_BACKOFF`       | `5s`                                                         | Retry backoff duration         |
| `LOG_LEVEL`           | `info`                                                       | Log level                      |

### Provider Quick Reference

| Provider     | Env API Key                      | Env Base URL                       | Default Base URL                                          |
| ------------ | -------------------------------- | ---------------------------------- | --------------------------------------------------------- |
| `openai`     | `OPENAI_API_KEY` → `LLM_API_KEY` | `OPENAI_BASE_URL` → `LLM_BASE_URL` | `https://api.openai.com/v1`                               |
| `gemini`     | `GEMINI_API_KEY`                 | `GEMINI_BASE_URL`                  | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `openrouter` | `OPENROUTER_API_KEY`             | `OPENROUTER_BASE_URL`              | `https://openrouter.ai/api/v1`                            |
| `lmstudio`   | _(none)_                         | `LMSTUDIO_BASE_URL`                | `http://localhost:1234/v1`                                |

## Retry & Recovery

Each pipeline stage is executed as a River job, providing built-in:

- **Automatic retries** (configurable via `--max-retries` / `MAX_RETRIES`)
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
| River worker pool            |            5             | `MaxWorkers: 5` in queue config    |

The indexer uses `golang.org/x/sync/errgroup` with a bounded semaphore to process files in parallel. The eval pipeline is sequential by design — the rate limit (API's 429 + `Retry-After`) is the bottleneck, not CPU, so concurrency can't improve throughput.

## Project Structure

```
cmd/
  preprocess/main.go     — Preprocessing CLI (River job trigger)
  index/main.go          — Indexing CLI (River job trigger)
  eval/main.go           — Evaluation CLI (River job trigger)
  query/main.go          — Interactive/one-shot query CLI (sync)
  workerd/main.go        — River worker daemon (all workers)

internal/
  config/config.go       — Configuration (flags + env vars)
  db/
    db.go                — PG connection pool
    migrate.go           — Schema migrations + River auto-migrate
    migrations/          — SQL migration files
  types/
    document.go          — Document type
    pipeline.go          — Stage, StageResult types
    indexing.go          — Chunk, Embedding, DocumentChunk types
    workflow.go          — Workflow, WorkflowStep types
    eval.go              — EvalQuestion, RetrievalResult, AggregateMetrics, EvalReport
    query.go             — SearchResult type
  workflow/
    store.go             — Workflow/step CRUD + runStep helper
    poll.go              — PollUntilDone helper
    preprocess_worker.go — Clone, Preprocess, Verify workers
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
  stage/
    clone.go             — Git clone/pull stage
    preprocess.go        — Preprocess pipeline stage
    verify.go            — Verification stage

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
| `stage/`        | Clone, preprocess, verify stage execution                                                                                                 |
| `workflow/`     | Store CRUD, status transitions, state merging, Kind() methods                                                                             |
| `db/`           | Migration version parsing                                                                                                                 |

## License

MIT
