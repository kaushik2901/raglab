# GitLab Handbook RAG Pipeline

Preprocessing + indexing pipeline for GitLab's public handbook (~4,500 markdown files). Converts Hugo-based documentation into clean, LLM/RAG-friendly markdown and indexes it into a Qdrant vector store.

Uses **River** (`github.com/riverqueue/river`) as the job engine — a lightweight Go queue backed by Postgres for durable at-least-once execution, retries with backoff, and concurrency control.

## Architecture

```
┌─────────────────────┐     ┌──────────────────────┐
│  cmd/preprocess     │     │  cmd/index           │
│  (River job insert) │     │  (River job insert)  │
└─────────┬───────────┘     └──────────┬───────────┘
          │                            │
          ▼                            ▼
┌────────────────────────────────────────────────────┐
│                  Postgres (River)                  │
│  workflows  │  workflow_steps  │  river_job  │ ... │
└─────────┬────────────────────────────────┬─────────┘
          │                                │
          ▼                                ▼
┌──────────────────────┐     ┌──────────────────────────┐
│  cmd/workerd         │     │  cmd/workerd             │
│  Clone → Preprocess  │     │  Parse → Chunk → Embed   │
│  → Verify            │     │  → Store                 │
│  (3 River workers)   │     │  (4 River workers)       │
└──────────────────────┘     └──────────────────────────┘
```

Two independent pipelines share a single `workerd` process. The preprocessing pipeline writes cleaned markdown to disk; the indexing pipeline reads it back and stores vectors in Qdrant.

## Prerequisites

```bash
# Start Postgres + Qdrant
docker compose up -d
```

## Preprocessing Pipeline

Transforms Hugo markdown into clean markdown suitable for LLM ingestion.

### Stages

1. **clone** — clones the handbook repo (or pulls latest if already present)
2. **preprocess** — transforms each markdown file:
   - Resolves `{{% include "path" %}}` directives (recursive with cycle detection)
   - Strips Hugo shortcodes (details, alert, panel, youtube, etc.)
   - Cleans raw HTML (style, script, iframe, img, a, table, div, etc.)
   - Resolves `{{< ref >}}` / `{{< relref >}}` to markdown links
3. **verify** — validates output quality (file count, directory structure, no stray shortcodes/HTML, minimum content size, total size sanity)

### Usage

```bash
# Build
go build -o bin\preprocess.exe .\cmd\preprocess

# Start River worker daemon (in a separate terminal)
go build -o bin\workerd.exe .\cmd\workerd
.\bin\workerd.exe

# Run (clones handbook, preprocesses, verifies)
.\bin\preprocess.exe

# Or via make.cmd
make.cmd          # build
make.cmd run      # build & run all
make.cmd clean    # remove bin/ artifacts/
make.cmd test     # run all tests
```

### CLI Flags

| Flag             | Env Var        | Default                                                | Description                                      |
| ---------------- | -------------- | ------------------------------------------------------ | ------------------------------------------------ |
| `--repo-url`     | `REPO_URL`     | `https://gitlab.com/gitlab-com/content-sites/handbook` | Handbook repository URL                          |
| `--tag`          | `TAG`          | `pre-<timestamp>`                                      | Workflow tag (artifacts stored under this)       |
| `--include-dirs` | `INCLUDE_DIRS` | `""`                                                   | Comma-separated subdirs to process (empty = all) |

Artifacts are stored at `artifacts/preprocessing/<tag>/repo/` and `artifacts/preprocessing/<tag>/output/`.

## Indexing Pipeline

Builds on the preprocessing output. Reads cleaned markdown, chunks documents, generates embeddings, and stores vectors in Qdrant.

### Stages

1. **parse** — reads all `.md` files from the preprocessed output directory
2. **chunk** — splits documents into fixed-size word windows with configurable overlap
3. **embed** — sends chunks to any OpenAI-compatible embedding API (OpenAI, OpenRouter, LM Studio, Ollama)
4. **store** — upserts document chunks with embeddings into Qdrant via gRPC

### Usage

```bash
# Build
go build -o bin\index.exe .\cmd\index

# Start River worker daemon (must be running)
.\bin\workerd.exe

# Run (requires preprocessed output tag)
.\bin\index.exe --input-tag pre-20260603-141651
```

### CLI Flags

| Flag                | Env Var           | Default                  | Description                                    |
| ------------------- | ----------------- | ------------------------ | ---------------------------------------------- |
| `--input-tag`       | `INPUT_TAG`       | —                        | **Required.** Preprocessed output tag to index |
| `--tag`             | `TAG`             | `idx-<timestamp>`        | Workflow tag                                   |
| `--chunk-strategy`  | `CHUNK_STRATEGY`  | `fixed`                  | Chunking strategy (fixed only)                 |
| `--chunk-size`      | `CHUNK_SIZE`      | `512`                    | Target token count per chunk                   |
| `--chunk-overlap`   | `CHUNK_OVERLAP`   | `64`                     | Token overlap between chunks                   |
| `--embedding-model` | `EMBEDDING_MODEL` | `text-embedding-3-small` | Embedding model name                           |
| `--batch-size`      | `BATCH_SIZE`      | `20`                     | Embedding API batch size                       |

Secrets and connection strings are read from environment variables only (not CLI flags):

| Env Var          | Default                                                 | Description                    |
| ---------------- | ------------------------------------------------------- | ------------------------------ |
| `LLM_BASE_URL`   | `https://api.openai.com/v1`                             | OpenAI-compatible API base URL |
| `LLM_API_KEY`    | `""`                                                    | API key                        |
| `QDRANT_URL`     | `http://localhost:6334`                                 | Qdrant gRPC endpoint           |
| `QDRANT_API_KEY` | `""`                                                    | Qdrant API key (optional)      |
| `DATABASE_URL`   | `postgres://rag:rag@localhost:5432/rag?sslmode=disable` | Postgres connection string     |

### Provider Configuration

| Provider   | `LLM_BASE_URL`                 | `LLM_API_KEY` |
| ---------- | ------------------------------ | ------------- |
| OpenAI API | `https://api.openai.com/v1`    | Required      |
| OpenRouter | `https://openrouter.ai/api/v1` | Required      |
| LM Studio  | `http://localhost:1234/v1`     | Empty         |
| Ollama     | `http://localhost:11434/v1`    | Empty         |

## Retry & Recovery

Each stage is executed as a River job, providing built-in:

- **Automatic retries** (configurable via `--max-retries` / `MAX_RETRIES`)
- **Exponential backoff** with jitter
- **At-least-once execution** — workers are retried on failure
- **Durable state** — workflow and step progress is persisted in Postgres

The `workerd` process must be running to execute jobs. If it crashes, pending jobs survive and are picked up on restart.

## Project Structure

```
cmd/
  preprocess/main.go          — Preprocessing CLI (River job trigger)
  index/main.go               — Indexing CLI (River job trigger)
  workerd/main.go             — River worker daemon (all 7 workers)

internal/
  config/config.go            — Configuration (flags + env vars)
  db/
    db.go                     — PG connection pool
    migrate.go                — Schema migrations + River auto-migrate
    migrations/               — SQL migration files
  types/
    document.go               — Document type
    pipeline.go               — Stage, StageResult types
    indexing.go               — Chunk, Embedding, DocumentChunk types
    workflow.go               — Workflow, WorkflowStep types
  workflow/
    store.go                  — Workflow/step CRUD + runStep helper
    poll.go                   — PollUntilDone helper
    preprocess_worker.go      — Clone, Preprocess, Verify workers
    index_worker.go           — Parse, Chunk, Embed, Store workers
  preprocessor/
    includes.go               — {{% include %}} resolver
    shortcodes.go             — Shortcode stripper with rules engine
    html.go                   — HTML cleaning
    refs.go                   — {{< ref >}} / {{< relref >}} resolver
    preprocessor.go           — Orchestrator (applies all transforms)
  parser/parser.go            — Reads cleaned .md files into Documents
  chunker/
    chunker.go                — Chunker interface
    fixed.go                  — Fixed-size word-window chunking
  embedder/
    embedder.go               — Embedder interface
    openai.go                 — OpenAI-compatible HTTP embedder
  store/
    store.go                  — VectorStore interface
    qdrant.go                 — Qdrant gRPC backend
  stage/
    clone.go                  — Git clone/pull stage
    preprocess.go             — Preprocess pipeline stage
    verify.go                 — Verification stage
    parse.go                  — Indexing parse stage
    chunk.go                  — Indexing chunk stage
    embed.go                  — Indexing embed stage

docs/
  river-implementation-plan.md — River workflow implementation plan
  project-vision-and-roadmap.md

docker-compose.yml            — Postgres 16 + Qdrant
```

## Testing

```bash
go test ./...
```

Tests requiring a Qdrant or Postgres server use `t.Skip(...)` and are excluded from `go test ./...` by default.

## License

MIT
