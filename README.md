# Gitlab Handbook RAG Pipeline

Preprocessing + indexing pipeline for GitLab's public handbook (~4,500 markdown files). Converts Hugo-based documentation into clean, LLM/RAG-friendly markdown and optionally indexes it into a Qdrant vector store.

## Preprocessing Pipeline

Transforms Hugo markdown into clean markdown suitable for LLM ingestion.

### Stages

1. **clone** — clones the handbook repo (or pulls latest if already present)
2. **preprocess** — transforms each markdown file:
   - Resolves `{{% include "path" %}}` directives (recursive with cycle detection)
   - Strips Hugo shortcodes (details, alert, panel, youtube, member-by-name, etc.)
   - Cleans raw HTML (style, script, iframe, img, a, table, div, etc.)
   - Resolves `{{< ref >}}` / `{{< relref >}}` to markdown links
3. **verify** — validates output quality (file count, directory structure, no stray shortcodes/HTML, minimum content size, total size sanity)

### Usage

```bash
# Build
go build -o bin\preprocess.exe .\cmd\preprocess

# Run (clones handbook, preprocesses, verifies)
bin\preprocess.exe

# Or use the build script (Windows)
make.cmd          # build
make.cmd run      # build & run
make.cmd clean    # remove bin/ output/ .journal/
make.cmd test     # run all tests
```

### CLI Flags

| Flag              | Env Var         | Default                                                | Description                                      |
| ----------------- | --------------- | ------------------------------------------------------ | ------------------------------------------------ |
| `--repo-url`      | `REPO_URL`      | `https://gitlab.com/gitlab-com/content-sites/handbook` | Handbook repository URL                          |
| `--repo-path`     | `REPO_PATH`     | `./handbook`                                           | Local clone path                                 |
| `--output`        | `OUTPUT_PATH`   | `./output`                                             | Cleaned markdown output directory                |
| `--include-dirs`  | `INCLUDE_DIRS`  | `""`                                                   | Comma-separated subdirs to process (empty = all) |
| `--max-retries`   | `MAX_RETRIES`   | `3`                                                    | Max retries per stage on failure                 |
| `--retry-backoff` | `RETRY_BACKOFF` | `5s`                                                   | Initial retry backoff duration                   |
| `--log-level`     | `LOG_LEVEL`     | `info`                                                 | Log level (debug/info/warn)                      |
| `--from`          | —               | —                                                      | Resume from a specific stage name                |

### Examples

```bash
# Custom output directory
bin\preprocess.exe --output .\clean-handbook

# Resume from preprocess stage
bin\preprocess.exe --from preprocess

# Fewer retries for quick testing
bin\preprocess.exe --max-retries 1 --retry-backoff 1s

# Only process specific subdirectories
bin\preprocess.exe --include-dirs handbook,company,jobs
```

## Indexing Pipeline

Builds on the preprocessing output. Reads cleaned markdown, chunks documents, generates embeddings, and stores vectors in Qdrant.

### Stages

1. **parse** — walks `--output` dir, reads all `.md` files into `[]types.Document`
2. **chunk** — splits documents into fixed-size word windows with configurable overlap
3. **embed** — sends chunks to any OpenAI-compatible embedding API (OpenAI, OpenRouter, LM Studio, Ollama)
4. **store** — upserts document chunks with embeddings into Qdrant via gRPC

### Prerequisites

```bash
# Start Qdrant
docker compose up -d
```

### Usage

```bash
# Build
go build -o bin\index.exe .\cmd\index

# Run (requires preprocessed output in --output dir)
bin\index.exe

# Or via make.cmd
make.cmd build-index
make.cmd run-index
make.cmd clean-index
```

### CLI Flags (additional)

| Flag                | Env Var           | Default                     | Description                                      |
| ------------------- | ----------------- | --------------------------- | ------------------------------------------------ |
| `--chunk-strategy`  | `CHUNK_STRATEGY`  | `fixed`                     | Chunking strategy (fixed only)                   |
| `--chunk-size`      | `CHUNK_SIZE`      | `512`                       | Target token count per chunk                     |
| `--chunk-overlap`   | `CHUNK_OVERLAP`   | `64`                        | Token overlap between chunks                     |
| `--embedding-model` | `EMBEDDING_MODEL` | `text-embedding-3-small`    | Embedding model name                             |
| `--batch-size`      | `BATCH_SIZE`      | `20`                        | Embedding API batch size                         |
| `--llm-base-url`    | `LLM_BASE_URL`    | `https://api.openai.com/v1` | OpenAI-compatible API base URL                   |
|                     | `LLM_API_KEY`     | `""`                        | API key (empty for local servers like LM Studio) |
|                     | `QDRANT_URL`      | `http://localhost:6334`     | Qdrant gRPC endpoint                             |
|                     | `QDRANT_API_KEY`  | `""`                        | Qdrant API key (optional)                        |

Connection strings (`LLM_API_KEY`, `QDRANT_URL`, `QDRANT_API_KEY`) are read from environment variables only (not CLI flags) to keep secrets out of process listings.

### Provider Configuration

| Provider   | `LLM_BASE_URL`                 | `LLM_API_KEY` |
| ---------- | ------------------------------ | ------------- |
| OpenAI API | `https://api.openai.com/v1`    | Required      |
| OpenRouter | `https://openrouter.ai/api/v1` | Required      |
| LM Studio  | `http://localhost:1234/v1`     | Empty         |
| Ollama     | `http://localhost:11434/v1`    | Empty         |

### Resume

Both pipelines support `--from <stage>` to skip completed stages:

```bash
bin\index.exe --from embed       # skip parse + chunk
bin\preprocess.exe --from verify # skip clone + preprocess
```

Each pipeline maintains its own journal (`.journal/` for preprocess, `.journal-index/` for index) so they are independently resumable.

## Testing

```bash
go test ./...
```

Tests requiring a Qdrant server use `t.Skip("requires Qdrant server")` and are excluded from `go test ./...` by default.

## Project Structure

```
cmd/
  preprocess/main.go          — Preprocessing CLI
  index/main.go               — Indexing CLI

internal/
  config/config.go            — Configuration (flags + env vars)
  types/
    document.go               — Document type
    pipeline.go               — StageID, StageRecord, StageResult
    indexing.go               — Chunk, Embedding, DocumentChunk types
  journal/journal.go          — Journal interface + gob-backed implementation
  pipeline/pipeline.go        — Generic pipeline runner (retry, cache, resume)
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
    store.go                  — Indexing store stage

docker-compose.yml            — Qdrant service for local development
```

## License

MIT
