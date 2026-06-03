# Indexing Pipeline — Implementation Plan

## Context Summary

| Metric                 | Value                          |
| ---------------------- | ------------------------------ |
| Cleaned markdown files | ~4,496 (in `output/`)          |
| Total content size     | ~44 MB                         |
| Chunking strategies    | 3 (fixed, semantic, recursive) |
| Embedding backends     | 1 (OpenAI-compatible)          |
| Vector store backends  | 1 (Qdrant)                     |
| Pipeline stages        | 4 (parse, chunk, embed, store) |

## Project Structure — Additions for Indexing

```
root/
│
├── cmd/
│   ├── preprocess/               # ✓ Existing
│   └── index/                    # NEW: Indexing CLI
│       └── main.go
│
├── internal/
│   ├── pipeline/                 # ✓ Existing (reused as-is)
│   ├── journal/                  # ✓ Existing (reused as-is)
│   │
│   ├── config/                   # Extended: new fields for indexing
│   │   └── config.go
│   │
│   ├── types/                    # Extended: Chunk, Embedding types
│   │   ├── document.go           # ✓ Existing
│   │   ├── pipeline.go           # ✓ Existing
│   │   └── indexing.go           # NEW: Chunk, Embedding, DocumentChunk
│   │
│   ├── stage/                    # New stages added here
│   │   ├── clone.go              # ✓ Existing
│   │   ├── sync_data.go          # ✓ Existing
│   │   ├── preprocess.go         # ✓ Existing
│   │   ├── verify.go             # ✓ Existing
│   │   ├── parse.go              # NEW: read cleaned markdown → Documents
│   │   ├── chunk.go              # NEW: Documents → Chunks
│   │   ├── embed.go              # NEW: Chunks → Embeddings
│   │   └── store.go              # NEW: Embeddings → Vector DB
│   │
│   ├── parser/                   # NEW: document parsing
│   │   └── parser.go             # Read markdown files, return Documents
│   │
│   ├── chunker/                  # NEW: chunking strategies
│   │   ├── chunker.go            # Chunker interface
│   │   ├── fixed.go              # Fixed-size token chunking
│   │   ├── semantic.go           # Section-based (heading) chunking
│   │   └── recursive.go          # Recursive character splitting
│   │
│   ├── embedder/                 # NEW: embedding model abstraction
│   │   ├── embedder.go           # Embedder interface
│   │   └── openai.go             # Single impl — works with any OpenAI-compatible endpoint
│   │
│   └── store/                    # NEW: vector store abstraction
│       ├── store.go              # VectorStore interface
│       └── qdrant.go             # Qdrant backend
│
├── go.mod                        # Updated with new dependencies
└── make.cmd                      # Extended with index command
```

## Domain Types

```go
// internal/types/indexing.go

// Chunk represents a single piece of a document after chunking.
type Chunk struct {
    ID           string            // unique identifier (UUID or hash-based)
    DocumentPath string            // source document relative path
    Content      string            // chunk text content
    Metadata     map[string]string // optional: heading path, section, etc.
    TokenCount   int               // approximate token count
    Index        int               // chunk index within document (0-based)
}

// Embedding represents a vector embedding for a chunk.
type Embedding struct {
    ChunkID     string
    Vector      []float64
    Model       string    // model name used for embedding
    Dimensions  int
}

// DocumentChunk links a chunk with its embedding for storage.
type DocumentChunk struct {
    Chunk     Chunk
    Embedding Embedding
}
```

## Stage Definitions

### Stage 1: Parse (`internal/stage/parse.go`)

**Input:** Cleaned markdown directory (e.g., `./output/`)
**Output:** List of `types.Document` in pipeline state
**Idempotency key:** "parse" + content hash of the input directory

**Logic:**

1. Walk `cfg.OutputPath` recursively for `.md` files
2. For each file, create a `types.Document` with its relative path, content, and size
3. Store `[]types.Document` in state as `"documents"`

**Edge cases:**

- Empty output directory → return zero documents (warning)
- Binary/non-markdown files → skip
- Very large files → read and process as normal (no size limit)

---

### Stage 2: Chunk (`internal/stage/chunk.go`)

**Input:** `[]types.Document` from state
**Output:** `[]types.Chunk` in pipeline state
**Idempotency key:** "chunk" + chunk_strategy + chunk_size + chunk_overlap

**Logic:**

1. Read `chunk_strategy` from config (fixed / semantic / recursive)
2. Instantiate the appropriate chunker
3. For each document, call `chunker.Chunk(doc)` → `[]types.Chunk`
4. Accumulate all chunks into state as `"chunks"`
5. Store chunk count in output

**Chunking strategies:**

| Strategy  | Description                                    | Best for                     |
| --------- | ---------------------------------------------- | ---------------------------- |
| Fixed     | Split by fixed token count with overlap        | Simple, uniform chunks       |
| Semantic  | Split on markdown headings (`#`, `##`, etc.)   | Section-aware retrieval      |
| Recursive | Recursive character splitting (like LangChain) | Balancing content boundaries |

---

### Stage 3: Embed (`internal/stage/embed.go`)

**Input:** `[]types.Chunk` from state
**Output:** `[]types.DocumentChunk` in pipeline state
**Idempotency key:** "embed" + model_name

**Logic:**

1. Read `cfg.EmbeddingModel` to select model name
2. Create `openai.Embedder` with `cfg.LLMBaseURL` and `cfg.LLMAPIKey`
3. Batch chunks (e.g., 20 at a time)
4. Generate embeddings for each batch
5. Pair chunks with their embeddings → `[]types.DocumentChunk`
6. Store in state as `"document_chunks"`

**Batching:** Send chunks in batches to avoid token limits. Default batch size: 20.

The embedder talks the OpenAI-compatible API format, which all supported providers speak:

| Provider          | `LLMBaseURL`                       | `LLMAPIKey`            |
| ----------------- | ---------------------------------- | ---------------------- |
| OpenAI API        | `https://api.openai.com/v1`        | Required               |
| OpenRouter        | `https://openrouter.ai/api/v1`     | Required               |
| LM Studio         | `http://localhost:1234/v1`         | Empty                  |
| Ollama            | `http://localhost:11434/v1`        | Empty                  |

---

### Stage 4: Store (`internal/stage/store.go`)

**Input:** `[]types.DocumentChunk` from state
**Output:** Write confirmation + stats in state
**Idempotency key:** "store" + vector_store_type

**Logic:**

1. Read `QDRANT_URL` and `QDRANT_API_KEY` from config
2. Connect to the Qdrant server via gRPC
3. Ensure collection exists with the correct vector size (depends on model)
4. Upsert all document chunks with embeddings
5. Return count of stored vectors

**Vector store backend:**

| Backend | Connection                              | Index type |
| ------- | --------------------------------------- | ---------- |
| Qdrant  | `QDRANT_URL` (default: http://localhost:6333) | HNSW  |

**Qdrant collection schema:**

```protobuf
collection: document_chunks
vector_size: 1536  // depends on embedding model
distance: Cosine
payload:
  - id: Text (PK)
  - document_path: Text
  - content: Text
  - token_count: Integer
  - chunk_index: Integer
  - model: Text
```

## Configuration Additions

```go
// Extended Config struct — new fields for indexing
type Config struct {
    // Existing fields (preprocessing)
    RepoURL      string
    RepoPath     string
    OutputPath   string
    MaxRetries   int
    RetryBackoff time.Duration
    LogLevel     string

    // New fields (indexing)
    ChunkStrategy  string        // fixed / semantic / recursive
    ChunkSize      int           // tokens per chunk (default: 512)
    ChunkOverlap   int           // overlap between chunks (default: 64)
    EmbeddingModel string        // model name, e.g. text-embedding-3-small
    BatchSize      int           // embedding batch size (default: 20)

    // Connection strings (env vars)
    LLMBaseURL     string        // LLM_BASE_URL
    LLMApiKey      string        // LLM_API_KEY (optional — empty for local servers)
    QdrantURL      string        // QDRANT_URL (default: http://localhost:6333)
    QdrantAPIKey   string        // QDRANT_API_KEY (optional)
}
```

## Pipeline Wiring (`cmd/index/main.go`)

```go
func buildIndexPipeline(cfg *config.Config) pipeline.Pipeline {
    return pipeline.Pipeline{
        Journal: journal.NewGobFileJournal(".journal-index"),
        Config:  cfg,
        Stages: []pipeline.Stage{
            // No clone/preprocess needed — reads from existing output/
            stagepkg.ParseStage(cfg),
            stagepkg.ChunkStage(cfg),
            stagepkg.EmbedStage(cfg),
            stagepkg.StoreStage(cfg),
        },
    }
}
```

The indexing pipeline consumes the output of the preprocessing pipeline directly. It does **not** depend on cloning or preprocessing — it reads from `cfg.OutputPath` (default: `./output`). This means:

- Preprocessing runs first: `preprocess` → produces `output/`
- Indexing runs second: `index` → reads `output/` → produces vector DB

Each pipeline maintains its own journal (`.journal/` vs `.journal-index/`) so they are independently resumable.

## Implementation Order

| Step | What                                                                         | Why                                    |
| ---- | ---------------------------------------------------------------------------- | -------------------------------------- |
| 1    | Extend `internal/types/indexing.go` — Chunk, Embedding, DocumentChunk types  | Foundation for all indexing code       |
| 2    | Extend `internal/config/config.go` — add indexing fields                     | CLIs and env bindings needed by stages |
| 3    | `internal/parser/parser.go` — walk output dir, read .md files into Documents | Data ingestion stage                   |
| 4    | `internal/chunker/chunker.go` — Chunker interface                            | Abstraction for pluggable strategies   |
| 5    | `internal/chunker/fixed.go` — fixed-size token chunking                      | Simplest strategy, good baseline       |
| 6    | `internal/chunker/semantic.go` — section-based chunking on headings          | Best for handbook structure            |
| 7    | `internal/chunker/recursive.go` — recursive character splitting              | Compatible with LangChain patterns     |
| 8    | `internal/stage/parse.go` — wire parser as pipeline stage                    | Stage 1 of indexing                    |
| 9    | `internal/stage/chunk.go` — wire chunker as pipeline stage                   | Stage 2 of indexing                    |
| 10   | `internal/embedder/embedder.go` — Embedder interface                         | Abstraction                      |
| 11   | `internal/embedder/openai.go` — OpenAI-compatible embedder                   | Single impl for all providers     |
| 12   | `internal/stage/embed.go` — wire embedder as pipeline stage                  | Stage 3 of indexing               |
| 13   | `internal/store/store.go` — VectorStore interface                            | Abstraction for pluggable backends|
| 14   | `internal/store/qdrant.go` — Qdrant backend                                  | Vector store with gRPC API       |
| 15   | `internal/stage/store.go` — wire store as pipeline stage                     | Stage 4 of indexing               |
| 16   | `cmd/index/main.go` — wire complete indexing pipeline CLI                    | Complete CLI flow                      |
| 17   | Extend `make.cmd` with index build/run commands                              | Developer workflow                     |
| 18   | Integration test on preprocessed handbook data                               | End-to-end verification                |

## Dependencies (go.mod)

New dependencies:

- `github.com/qdrant/go-client` — Qdrant gRPC client

The embedder stages use HTTP calls (standard library) — no additional deps needed.

## Testing Strategy

| Package                                    | Est. Tests | Key areas                                                   |
| ------------------------------------------ | ---------- | ----------------------------------------------------------- |
| `internal/parser`                          | 6          | File walking, non-md skipping, empty dir, large files       |
| `internal/chunker` (fixed)                 | 8          | Token counting, boundary conditions, overlap, empty doc     |
| `internal/chunker` (semantic)              | 8          | Heading splitting, no headings, nested headings, edge cases |
| `internal/chunker` (recursive)             | 6          | Separator priority, max size, no splits possible            |
| `internal/embedder` (openai)               | 6          | API call, batching, error handling, rate limiting, provider agnostic |
| `internal/store` (qdrant)                  | 4          | Collection creation, upsert, query, connection error        |
| `internal/stage` (parse/chunk/embed/store) | 12         | Stage wiring, state passing, error propagation              |
| `cmd/index`                                | 6          | CLI parsing, pipeline wiring, resume, config validation     |

## Phase Plan

```
Phase 1 (Types + Config)
  ├── 1.1 Extend types: Chunk, Embedding, DocumentChunk
  └── 1.2 Extend config: indexing fields + validation

Phase 2 (Parser + Chunkers — depends on Phase 1, parallelizable)
  ├── 2.1 Parser (internal/parser/parser.go)
  ├── 2.2 Chunker interface + fixed.go
  ├── 2.3 Semantic chunker
  └── 2.4 Recursive chunker

Phase 3 (Embedder — depends on Phase 1)
  ├── 3.1 Embedder interface
  └── 3.2 OpenAI-compatible embedder

Phase 4 (Store — depends on Phase 1)
  ├── 4.1 VectorStore interface
  └── 4.2 Qdrant backend

Phase 5 (Pipeline stages — depends on Phases 2, 3, 4)
  ├── 5.1 Parse stage
  ├── 5.2 Chunk stage
  ├── 5.3 Embed stage
  └── 5.4 Store stage

Phase 6 (CLI + Integration — depends on Phase 5)
  ├── 6.1 cmd/index/main.go
  ├── 6.2 make.cmd update
  └── 6.3 Integration test on real data
```

## Extensibility

The indexing pipeline follows the same patterns as the preprocessing pipeline:

- **Pluggable chunkers** — implement `Chunker` interface, register in config
- **Pluggable embedders** — implement `Embedder` interface, add alternative impl (unlikely needed — all providers use OpenAI-compatible API)
- **Pluggable stores** — implement `VectorStore` interface, add backend
- **Same pipeline runner** — journaling, retry, resume work unchanged

Adding a new chunking strategy:

```go
func init() {
    registry["my-strategy"] = &MyChunker{}
}
```

Adding a new vector store:

```go
type MyStore struct {}

func (s *MyStore) Connect(ctx context.Context, dsn string) error { ... }
func (s *MyStore) Store(ctx context.Context, chunks []types.DocumentChunk) error { ... }
func (s *MyStore) Close() error { ... }
```

This plan is designed to parallelize Phase 2 (chunkers) with Phase 3 (embedders) and Phase 4 (stores) since they depend only on Phase 1.
