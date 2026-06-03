# Indexing Pipeline — Phase-Wise Execution Plan

This document breaks the indexing pipeline into small, independent work items that can be picked up by separate agents. Each item specifies inputs, outputs, interface contracts, unit tests, and acceptance criteria. Tests are written alongside implementation within each phase.

---

## Dependency Graph

```
Phase 1 (Types + Config)
  ├── 1.1 Extend types: Chunk, Embedding, DocumentChunk
  └── 1.2 Extend config: indexing fields + validation
       │
Phase 2 (Parser + Chunkers — depends on Phase 1, parallelizable within phase)
  ├── 2.1 Parser (internal/parser/parser.go)
  ├── 2.2 Chunker interface + fixed.go
  ├── 2.3 Semantic chunker
  └── 2.4 Recursive chunker
       │
Phase 3 (Embedder — depends on Phase 1, parallelizable with Phase 2 & 4)
  ├── 3.1 Embedder interface
  └── 3.2 OpenAI-compatible embedder (single impl for all providers)
       │
Phase 4 (Store — depends on Phase 1, parallelizable with Phase 2 & 3)
  ├── 4.1 VectorStore interface
  └── 4.2 Qdrant backend
       │
Phase 5 (Pipeline stages — depends on Phases 2, 3, 4)
  ├── 5.1 Parse stage
  ├── 5.2 Chunk stage
  ├── 5.3 Embed stage
  └── 5.4 Store stage
       │
Phase 6 (CLI + Integration — depends on Phase 5)
  ├── 6.1 cmd/index/main.go
  ├── 6.2 make.cmd update
  └── 6.3 Integration test on real data
```

---

## Phase 1: Types & Config Extensions

### 1.1 — Extend Types (`internal/types/indexing.go`)

**Input:** Existing `internal/types/document.go`, `internal/types/pipeline.go`
**Output:** New file `internal/types/indexing.go`

**Types to define:**

```go
type Chunk struct {
    ID           string
    DocumentPath string
    Content      string
    Metadata     map[string]string
    TokenCount   int
    Index        int
}

type Embedding struct {
    ChunkID    string
    Vector     []float64
    Model      string
    Dimensions int
}

type DocumentChunk struct {
    Chunk     Chunk
    Embedding Embedding
}
```

**Acceptance criteria:**
- Package compiles cleanly with existing types
- All new types exported with correct fields
- `Chunk.ID` is a string (UUID/hash based, not typed UUID)
- `Embedding.Vector` is `[]float64` (not `[]float32`)
- Zero-value `Metadata` map is nil (not initialized)

**Unit tests (`internal/types/indexing_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestChunkCreation` | All Chunk fields set correctly |
| `TestChunkZeroValue` | Zero-value Chunk has empty strings, nil Metadata, zero ints |
| `TestChunkMetadataNil` | Default Metadata is nil, not empty map |
| `TestEmbeddingCreation` | All Embedding fields set correctly |
| `TestEmbeddingZeroValue` | Zero-value Embedding has nil Vector |
| `TestEmbeddingVectorType` | Vector is `[]float64`, append works |
| `TestDocumentChunkCreation` | Links Chunk and Embedding correctly |
| `TestDocumentChunkRoundTrip` | Assign Chunk and Embedding, verify fields accessible |

---

### 1.2 — Extend Config (`internal/config/config.go`)

**Input:** Existing `internal/config/config.go`
**Output:** Extended `Config` struct with new indexing fields + validation

**New fields to add:**

```go
type Config struct {
    // Existing fields (unchanged)
    RepoURL      string
    RepoPath     string
    OutputPath   string
    MaxRetries   int
    RetryBackoff time.Duration
    LogLevel     string

    // New indexing fields
    ChunkStrategy  string   // fixed / semantic / recursive
    ChunkSize      int      // tokens per chunk (default: 512)
    ChunkOverlap   int      // overlap between chunks (default: 64)
    EmbeddingModel string   // model name, e.g. text-embedding-3-small
    BatchSize      int      // embedding batch size (default: 20)

    // Connection strings (env vars only, no CLI flags)
    LLMBaseURL     string // LLM_BASE_URL (default: https://api.openai.com/v1)
    LLMApiKey      string // LLM_API_KEY (optional — empty for local servers)
    QdrantURL      string // QDRANT_URL (default: http://localhost:6333)
    QdrantAPIKey   string // QDRANT_API_KEY (optional)
}
```

**CLI flags to add:**
```
--chunk-strategy    (default: "fixed")
--chunk-size        (default: 512)
--chunk-overlap     (default: 64)
--embedding-model   (default: "text-embedding-3-small")
--batch-size        (default: 20)
--llm-base-url      (default: "https://api.openai.com/v1")
```

**Validation rules to add:**
- `ChunkStrategy` must be one of: `fixed`, `semantic`, `recursive`
- `ChunkSize` must be > 0
- `ChunkOverlap` must be >= 0 and < `ChunkSize`
- `EmbeddingModel` must be non-empty
- `BatchSize` must be > 0
- `LLMBaseURL` must be non-empty

**Acceptance criteria:**
- New CLI flags parsed correctly via `flag` package
- New env vars fall back to defaults (e.g. `CHUNK_STRATEGY`, `CHUNK_SIZE`)
- Validation rejects invalid strategy, negative chunk size, overlap >= chunk size, empty base URL
- Connection strings read from env vars only (no CLI flags for secrets)
- Backward compatible: existing fields unchanged, old configs still validate

**Unit tests (`internal/config/config_test.go` — additions):**

| Test | What it validates |
|------|------------------|
| `TestValidate_InvalidChunkStrategy` | Rejects strategy not in {fixed, semantic, recursive} |
| `TestValidate_ValidChunkStrategies` | All three strategies accepted |
| `TestValidate_ZeroChunkSize` | ChunkSize = 0 rejected |
| `TestValidate_NegativeChunkSize` | ChunkSize < 0 rejected |
| `TestValidate_NegativeChunkOverlap` | ChunkOverlap < 0 rejected |
| `TestValidate_OverlapGTEChunkSize` | Overlap >= Size rejected; overlap = size-1 accepted |
| `TestValidate_EmptyEmbeddingModel` | Empty model name rejected |
| `TestValidate_ZeroBatchSize` | BatchSize = 0 rejected |
| `TestValidate_NegativeBatchSize` | BatchSize < 0 rejected |
| `TestValidate_EmptyBaseURL` | Empty LLMBaseURL rejected |
| `TestValidate_DefaultsValid` | Default values pass validation |
| `TestChunkStrategyEnv` | Env var `CHUNK_STRATEGY` overrides default |
| `TestChunkSizeEnv` | Env var `CHUNK_SIZE` parsed as int |
| `TestChunkOverlapEnv` | Env var `CHUNK_OVERLAP` parsed as int |
| `TestBatchSizeEnv` | Env var `BATCH_SIZE` parsed as int |
| `TestLLMBaseURLEnv` | Env var `LLM_BASE_URL` read correctly |
| `TestQdrantURLEnv` | Env var `QDRANT_URL` read correctly |

---

## Phase 2: Parser & Chunkers

### 2.1 — Parser (`internal/parser/parser.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 2.2, 2.3, 2.4

**Input:** Directory path containing cleaned `.md` files
**Output:** `[]types.Document`

**Interface:**

```go
package parser

// ParseDir walks a directory recursively and returns all markdown documents.
// It skips non-.md files and hidden directories (starting with ".").
func ParseDir(dirPath string) ([]types.Document, error)

// ParseFile reads a single markdown file and returns a Document.
func ParseFile(filePath string, relPath string) (types.Document, error)
```

**Logic:**
1. Walk `dirPath` recursively using `filepath.WalkDir`
2. Filter files with `.md` extension
3. Skip hidden directories (starting with `.`)
4. For each file, read content, compute relative path, create `types.Document`
5. Return slice of documents

**Edge cases:**
- Empty directory → return empty slice (no error)
- No `.md` files → return empty slice (no error)
- File read error → return error with file path
- Symlinks → skip (don't follow)
- Hidden directories (`.git`, `.journal`) → skip
- Very large files → read as normal (no artificial limit)
- Binary files with `.md` extension → read as-is (caller responsibility)

**Acceptance criteria:**
- Walks directory recursively
- Returns correct relative paths (cleaned with forward slashes)
- Skips non-`.md` files
- Skips hidden directories
- Empty dir returns `[]types.Document{}, nil`
- Error on unreadable file propagated with file path

**Unit tests (`internal/parser/parser_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestParseDir_Basic` | Directory with multiple .md files returns all, correct paths/content |
| `TestParseDir_EmptyDir` | Empty directory returns empty slice, nil error |
| `TestParseDir_NoMdFiles` | Directory with only .txt files returns empty slice |
| `TestParseDir_SkipsHiddenDirs` | Files in `.hidden/` not included |
| `TestParseDir_SkipsNonMd` | `.txt`, `.json`, no-extension files skipped |
| `TestParseDir_Subdirectories` | Nested dirs included with correct relative paths |
| `TestParseDir_RelativePaths` | Paths use forward slashes, no `./` prefix, no backslashes on Windows |
| `TestParseDir_EmptyFileContent` | Empty `.md` file returns Document with empty Content |
| `TestParseDir_LargeFile` | File > 1MB read correctly, Content matches |
| `TestParseDir_NonExistentDir` | Returns error for non-existent directory |
| `TestParseDir_PermissionDenied` | Uses temp dir with restricted subdir, expect error on unreadable |
| `TestParseFile_Basic` | Single file returns correct Document |
| `TestParseFile_Error` | Non-existent file returns error |

---

### 2.2 — Chunker Interface + Fixed-Size Chunker (`internal/chunker/`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 2.1, 2.3, 2.4

**Input:** `types.Document`
**Output:** `[]types.Chunk`

**Chunker interface:**

```go
package chunker

type Chunker interface {
    // Chunk splits a document into chunks.
    Chunk(doc types.Document) ([]types.Chunk, error)
}
```

**Fixed chunker (`fixed.go`):**

```go
type FixedChunker struct {
    Size    int // target token count per chunk
    Overlap int // token overlap between adjacent chunks
}

func NewFixedChunker(size, overlap int) *FixedChunker

// estimateTokens approximates token count from character count.
// Uses len(text) / 4 as rough heuristic.
func estimateTokens(text string) int
```

**Logic:**
1. Split document content by whitespace into words
2. For each window of `Size` words, advance by `Size - Overlap` words
3. Create a `Chunk` with word-joined content and estimated token count
4. Set `Chunk.Index` sequentially (0-based)
5. Set `Chunk.DocumentPath` from document.Path
6. Generate `Chunk.ID` as `fmt.Sprintf("%s-chunk-%04d", doc.Path, index)`

**Edge cases:**
- Empty document → return empty slice
- Document shorter than chunk size → single chunk
- Overlap >= size → clamp to size - 1 (log warning)
- Single word document → single chunk
- Exact multiple → no partial chunk at end
- Whitespace-only content → empty chunk (or skip)

**Acceptance criteria:**
- Fixed-size splitting with configurable size and overlap
- Token estimation via character count heuristic
- Chunks ordered with correct Index
- Chunk IDs deterministic and unique
- Empty doc returns zero chunks
- Short doc returns one chunk

**Unit tests (`internal/chunker/fixed_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestFixedChunker_Basic` | 100-word doc, size=30, overlap=10 → correct chunk count, no gaps |
| `TestFixedChunker_NoOverlap` | Overlap=0 → non-overlapping windows |
| `TestFixedChunker_FullOverlap` | Overlap >= size → clamped to size-1 |
| `TestFixedChunker_EmptyDoc` | Empty content → empty slice |
| `TestFixedChunker_ShortDoc` | Doc shorter than size → single chunk |
| `TestFixedChunker_ExactMultiple` | Doc length is exact multiple of step → no extra chunk |
| `TestFixedChunker_SingleWord` | Single word → one chunk |
| `TestFixedChunker_WhitespaceOnly` | Only whitespace → zero chunks |
| `TestFixedChunker_ChunkIDs` | IDs match pattern `*-chunk-0000`, sequential |
| `TestFixedChunker_DocumentPath` | All chunks have correct DocumentPath |
| `TestFixedChunker_TokenCount` | TokenCount ~= len(content)/4 |
| `TestFixedChunker_MetadataNil` | Metadata is nil (not empty map) for plain text |

---

### 2.3 — Semantic Chunker (`internal/chunker/semantic.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 2.1, 2.2, 2.4

**Input:** `types.Document`
**Output:** `[]types.Chunk`

**Interface:**

```go
type SemanticChunker struct{}

func NewSemanticChunker() *SemanticChunker
```

**Logic:**
1. Split document content on markdown headings (`^# `, `^## `, `^### `, etc.)
2. Each heading + its content becomes one chunk
3. If no headings found, return entire document as single chunk
4. Heading text stored in `chunk.Metadata["heading"]`
5. Full heading path (e.g., "Installation > Configuration") built from parent headings
6. Set `chunk.Metadata["heading_path"]` for hierarchy

**Edge cases:**
- No headings → single chunk, empty metadata
- Document starts with text before first heading → that text is a chunk with empty heading
- Nested headings (e.g., `##` under `#`) → separate chunks, heading_path includes both
- Consecutive headings with no body text → empty content chunk (or skip)
- Only heading with no body → single chunk with heading text only
- Very deep nesting (6+ levels) → still splits on any `#`-prefixed line

**Acceptance criteria:**
- Splits on `# heading` lines (level 1-6)
- Metadata includes "heading" and "heading_path"
- No headings → single chunk, nil metadata
- Text before first heading → separate chunk with empty heading metadata
- Heading_path reflects hierarchy

**Unit tests (`internal/chunker/semantic_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestSemanticChunker_Basic` | Simple headings splits into correct chunks |
| `TestSemanticChunker_NoHeadings` | Plain text → single chunk |
| `TestSemanticChunker_EmptyDoc` | Empty content → single chunk with empty content |
| `TestSemanticChunker_TextBeforeFirstHeading` | Preamble text becomes its own chunk |
| `TestSemanticChunker_NestedHeadings` | `# A\n## B` → two chunks with correct heading_path |
| `TestSemanticChunker_ConsecutiveHeadings` | `## A\n## B\nbody` → B chunk includes only body |
| `TestSemanticChunker_HeadingLevels` | All 6 levels (`#` to `######`) split correctly |
| `TestSemanticChunker_HeadingPathHierarchy` | `# Install\n## Config\n### SSL` → paths build hierarchy |
| `TestSemanticChunker_H1Only` | Single `# Title` → one chunk with heading metadata |
| `TestSemanticChunker_ChunkIndices` | Indices are 0-based and sequential |
| `TestSemanticChunker_DocumentPath` | DocumentPath preserved from input |

---

### 2.4 — Recursive Chunker (`internal/chunker/recursive.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** 2.1, 2.2, 2.3

**Input:** `types.Document`
**Output:** `[]types.Chunk`

**Interface:**

```go
type RecursiveChunker struct {
    MaxSize     int // max characters per chunk
    Overlap     int // overlap in characters
    Separators  []string // ordered list of separators to split on
}

func NewRecursiveChunker(maxSize, overlap int) *RecursiveChunker
```

**Logic:**
1. Default separators: `["\n\n", "\n", ". ", " ", ""]`
2. For each chunk, try to split on the first separator that keeps chunks under MaxSize
3. If a split is still too large, recurse with the next separator in the list
4. Apply character-level overlap between adjacent chunks
5. Merge small chunks that end up below a minimum threshold

**Edge cases:**
- Content fits in one chunk → single chunk
- Single character separator needed (fallback to character split)
- Empty document → zero chunks
- Separator not found → fall through to next separator
- Very long word with no spaces → splits at MaxSize (last separator is "")

**Acceptance criteria:**
- Recursive splitting with ordered separator priority
- Configurable max size and overlap
- Fallback to character-level split when no separator found
- Chunks never exceed MaxSize
- Empty doc returns zero chunks

**Unit tests (`internal/chunker/recursive_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestRecursiveChunker_Basic` | Paragraph-separated text splits on `\n\n` |
| `TestRecursiveChunker_SingleChunk` | Short content fits in one chunk |
| `TestRecursiveChunker_EmptyDoc` | Empty content → empty slice |
| `TestRecursiveChunker_NoSeparatorFound` | Content with no spaces/periods/newlines → splits at MaxSize |
| `TestRecursiveChunker_SeparatorPriority` | Uses `\n\n` before `\n` before `. ` etc. |
| `TestRecursiveChunker_Overlap` | Adjacent chunks share overlap characters |
| `TestRecursiveChunker_CustomSeparators` | Custom separator list works correctly |
| `TestRecursiveChunker_MaxSizeEnforced` | No chunk exceeds MaxSize |
| `TestRecursiveChunker_SingleParagraph` | Long paragraph splits on `. ` then ` ` |
| `TestRecursiveChunker_ChunkIndices` | Sequential 0-based indices |
| `TestRecursiveChunker_DocumentPath` | DocumentPath preserved |
| `TestRecursiveChunker_TokenCount` | Token estimate populated for each chunk |

---

## Phase 3: Embedder

### 3.1 — Embedder Interface (`internal/embedder/embedder.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** Phase 2, Phase 4

**Input:** `[]types.Chunk`
**Output:** `[]types.Embedding`

**Interface:**

```go
package embedder

type Embedder interface {
    // Embed generates embeddings for a batch of chunks.
    Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error)

    // Dimensions returns the embedding vector dimensionality.
    Dimensions() int

    // ModelName returns the model name used for embeddings.
    ModelName() string
}
```

**Acceptance criteria:**
- Interface compiles cleanly
- Methods cover batch embedding, dimensionality query, model name
- Context is first parameter (Go convention)

**Unit tests are embedded in the implementation below — no standalone interface test needed.**

---

### 3.2 — OpenAI-Compatible Embedder (`internal/embedder/openai.go`)

**Depends on:** 3.1 (interface)
**Parallelizable with:** 2.x, 4.x

This is the **only** embedder implementation. All providers (OpenAI, OpenRouter, LM Studio, Ollama, vLLM, etc.) speak the same OpenAI-compatible wire format — only the `baseURL` and `apiKey` differ.

**Input:** `[]types.Chunk`, provider config
**Output:** `[]types.Embedding`

**Interface:**

```go
type Embedder struct {
    baseURL   string // e.g. https://api.openai.com/v1
    apiKey    string // may be empty for local servers
    model     string // embedding model name
    batchSize int
    client    *http.Client
}

func New(baseURL, apiKey, model string, batchSize int) *Embedder
```

**Logic:**
1. Build HTTP POST to `{baseURL}/embeddings`
2. Send chunks in batches of `batchSize`
3. Parse JSON response, extract vectors
4. Create `types.Embedding` for each chunk with model name, dimensions, vector
5. On 429 (rate limit), wait and retry with exponential backoff
6. On 4xx/5xx, return error

**Request format:**
```json
{
    "model": "text-embedding-3-small",
    "input": ["chunk1 text", "chunk2 text"]
}
```

**Response format:**
```json
{
    "data": [
        {"index": 0, "embedding": [0.001, ...]},
        {"index": 1, "embedding": [0.002, ...]}
    ],
    "model": "text-embedding-3-small"
}
```

**Edge cases:**
- Empty chunk list → return empty embeddings (no API call)
- API returns error → propagate with context
- API returns fewer embeddings than requested → error (mismatch)
- Network timeout → error (caller retry handles it)
- Rate limiting (429) → retry with backoff
- Empty chunk content → API accepts empty string, vector returned

**Acceptance criteria:**
- Sends batched HTTP requests to any OpenAI-compatible endpoint
- Works with OpenAI, OpenRouter, LM Studio, Ollama, etc. by just changing `baseURL`
- Returns embeddings for all input chunks
- Model name and dimensions populated in Embedding struct
- Rate limiting handled with backoff
- Empty input returns empty output (no API call)

**Provider configuration (configured via env vars / flags, no code changes needed):**

| Provider          | `LLM_BASE_URL`                    | `LLM_API_KEY`      |
| ----------------- | ---------------------------------- | ------------------ |
| OpenAI API        | `https://api.openai.com/v1`        | Required           |
| OpenRouter        | `https://openrouter.ai/api/v1`     | Required           |
| LM Studio         | `http://localhost:1234/v1`         | Empty              |
| Ollama            | `http://localhost:11434/v1`        | Empty              |

**Unit tests (`internal/embedder/openai_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestEmbed_Basic` | Mock server returns valid response, embeddings extracted correctly |
| `TestEmbed_Batching` | 25 chunks with batchSize=10 → 3 API calls |
| `TestEmbed_EmptyInput` | Empty chunk slice → empty embeddings, no HTTP call |
| `TestEmbed_ModelName` | Embedding.Model matches configured model |
| `TestEmbed_Dimensions` | Embedding.Dimensions matches vector length |
| `TestEmbed_ChunkID` | Embedding.ChunkID matches original Chunk.ID |
| `TestEmbed_APIClient` | Request sent with correct URL, headers, body |
| `TestEmbed_APIBadStatus` | API returns 500 → error |
| `TestEmbed_RateLimit` | API returns 429 then 200 → retries and succeeds |
| `TestEmbed_RateLimitExhausted` | Persistent 429 → error after retries |
| `TestEmbed_ResponseMismatch` | API returns fewer embeddings → error |
| `TestEmbed_ModelField` | API response model field used if present, falls back to configured |
| `TestEmbed_EmptyChunkContent` | Empty string chunk content handled cleanly |
| `TestNewEmbedder_Defaults` | Default baseURL and model set correctly |

**Test dependency:** `net/http/httptest` for mock server.

---

## Phase 4: Store

### 4.1 — VectorStore Interface (`internal/store/store.go`)

**Depends on:** Phase 1 (types)
**Parallelizable with:** Phase 2, Phase 3

**Interface:**

```go
package store

type VectorStore interface {
    // Connect establishes a connection to the vector store.
    Connect(ctx context.Context, dsn string) error

    // EnsureCollection creates the collection if it doesn't exist.
    // vectorSize and distance are fixed per collection.
    EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error

    // Store upserts document chunks with their embeddings.
    Store(ctx context.Context, chunks []types.DocumentChunk) error

    // Close terminates the connection.
    Close() error
}
```

**Acceptance criteria:**
- Interface compiles cleanly
- All methods take context as first parameter
- Covers lifecycle: Connect → EnsureCollection → Store → Close

---

### 4.2 — Qdrant Backend (`internal/store/qdrant.go`)

**Depends on:** 4.1 (interface)
**Parallelizable with:** 2.x, 3.x

**Input:** Qdrant gRPC connection config + `[]types.DocumentChunk`
**Output:** Vectors stored in Qdrant collection

**Interface:**

```go
type QdrantStore struct {
    client *qdrant.Client // from github.com/qdrant/go-client
}

func NewQdrantStore() *QdrantStore
```

**Logic:**
1. `Connect`: Dial Qdrant gRPC endpoint with API key (if provided)
2. `EnsureCollection`: Check if collection exists; if not, create with correct vector size and Cosine distance using HNSW config
3. `Store`: Convert `DocumentChunk` list to Qdrant points, upsert in batches of 100
4. `Close`: Close gRPC connection

**Point conversion:**
```go
func toPoint(doc types.DocumentChunk) *qdrant.PointStruct {
    return &qdrant.PointStruct{
        Id:      qdrant.NewID(doc.Chunk.ID),
        Vectors: qdrant.NewVectors(doc.Embedding.Vector...),
        Payload: qdrant.NewValueMap(map[string]any{
            "document_path": doc.Chunk.DocumentPath,
            "content":       doc.Chunk.Content,
            "token_count":   doc.Chunk.TokenCount,
            "chunk_index":   doc.Chunk.Index,
            "model":         doc.Embedding.Model,
        }),
    }
}
```

**Edge cases:**
- Connection refused → error
- Collection already exists → no-op (not error)
- Empty chunks list → return nil (no API call)
- gRPC timeout → error (caller retry)
- Invalid API key → authentication error
- Duplicate point IDs → upsert replaces existing

**Acceptance criteria:**
- Connects to Qdrant via gRPC
- Creates collection with correct vector size and Cosine distance
- Upserts document chunks as points with ID, vector, payload
- Empty input is no-op
- Close cleans up connection

**Unit tests (`internal/store/qdrant_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestQdrant_Connect` | Connect with valid DSN succeeds (mock or skip if no server) |
| `TestQdrant_ConnectRefused` | Connection to invalid address returns error |
| `TestQdrant_EnsureCollection_New` | Creates collection if not exists |
| `TestQdrant_EnsureCollection_Exists` | Existing collection → no error, no create call |
| `TestQdrant_Store_Basic` | Store single DocumentChunk → point upserted |
| `TestQdrant_Store_EmptyList` | Empty list → no API call |
| `TestQdrant_Store_Batching` | 250 chunks in batch of 100 → 3 upsert calls |
| `TestQdrant_Store_PointConversion` | Point fields match DocumentChunk: ID, vector, payload |
| `TestQdrant_Close` | Close called on connected store → no error |
| `TestQdrant_Close_Idempotent` | Double close → no error |
| `TestQdrant_ToPoint_ID` | Point ID matches Chunk.ID |
| `TestQdrant_ToPoint_Vector` | Point vector matches embedding vector |
| `TestQdrant_ToPoint_Payload` | Payload has all required fields: document_path, content, token_count, chunk_index, model |
| `TestQdrant_ToPoint_PayloadTypes` | Payload field types match schema (string, int, etc.) |

**Note:** Tests requiring a real Qdrant connection should use build tags (e.g., `//go:build integration`) and be excluded from `go test ./...` by default. Unit tests for `toPoint` conversion can run without a server. The Connect/EnsureCollection/Store happy-path tests should use `t.Skip("requires Qdrant server")` when no server is available, with a clear skip message.

---

## Phase 5: Pipeline Stages

### 5.1 — Parse Stage (`internal/stage/parse.go`)

**Depends on:** 2.1 (parser), Phase 1
**Parallelizable with:** 5.2, 5.3, 5.4

**Input:** `cfg.OutputPath` from config
**Output:** `state["documents"]` — `[]types.Document`

```go
func ParseStage(cfg *config.Config) pipeline.Stage
```

**Logic:**
1. Call `parser.ParseDir(cfg.OutputPath)`
2. Store result in state as `"documents"`
3. Return count in stage output as `"document_count"`

**Acceptance criteria:**
- Reads `cfg.OutputPath` as document source
- Stores `[]types.Document` in state under `"documents"`
- Returns document count in output
- Empty directory → zero documents, no error

**Unit tests (`internal/stage/parse_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestParseStage_Basic` | Stage returns correct document count in output |
| `TestParseStage_StateKey` | `state["documents"]` is `[]types.Document` with correct values |
| `TestParseStage_EmptyDir` | Empty output dir → 0 documents, no error |
| `TestParseStage_Error` | Non-existent dir → error returned from stage |

---

### 5.2 — Chunk Stage (`internal/stage/chunk.go`)

**Depends on:** 2.2, 2.3, 2.4 (chunkers), Phase 1
**Parallelizable with:** 5.1, 5.3, 5.4

**Input:** `state["documents"]` + `cfg.ChunkStrategy`, `cfg.ChunkSize`, `cfg.ChunkOverlap`
**Output:** `state["chunks"]` — `[]types.Chunk`

```go
func ChunkStage(cfg *config.Config) pipeline.Stage
```

**Logic:**
1. Read `cfg.ChunkStrategy` to pick chunker
2. Read `state["documents"]` (assert as `[]types.Document`)
3. For each doc, call `chunker.Chunk(doc)`
4. Accumulate all chunks into `state["chunks"]`
5. Return `"chunk_count"` in output

**Acceptance criteria:**
- Selects correct chunker based on strategy config
- Chunks all documents, accumulates into single slice
- Returns total chunk count

**Unit tests (`internal/stage/chunk_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestChunkStage_Basic` | Fixed chunker with valid config → chunks in state |
| `TestChunkStage_StrategySelection` | Each strategy (fixed/semantic/recursive) selected correctly |
| `TestChunkStage_StateKey` | `state["chunks"]` is `[]types.Chunk` |
| `TestChunkStage_ChunkCount` | Output "chunk_count" matches actual count |
| `TestChunkStage_EmptyDocuments` | No documents → zero chunks |
| `TestChunkStage_InvalidStrategy` | Unknown strategy → error |

---

### 5.3 — Embed Stage (`internal/stage/embed.go`)

**Depends on:** 3.2 (embedder), Phase 1
**Parallelizable with:** 5.1, 5.2, 5.4

**Input:** `state["chunks"]` + `cfg.EmbeddingModel`, `cfg.LLMBaseURL`, `cfg.LLMApiKey`, `cfg.BatchSize`
**Output:** `state["document_chunks"]` — `[]types.DocumentChunk`

```go
func EmbedStage(cfg *config.Config) pipeline.Stage
```

**Logic:**
1. Create embedder with `embedder.New(cfg.LLMBaseURL, cfg.LLMApiKey, cfg.EmbeddingModel, cfg.BatchSize)`
2. Read `state["chunks"]` (assert as `[]types.Chunk`)
3. Call `embedder.Embed(ctx, chunks)`
4. Pair chunks with embeddings → `[]types.DocumentChunk`
5. Store in `state["document_chunks"]`
6. Return `"embedding_count"` in output

**Acceptance criteria:**
- Creates the single embedder with correct config
- Returns embeddings for all chunks
- DocumentChunk pairs chunk with its embedding correctly
- Empty API key is accepted (for local servers like LM Studio)

**Unit tests (`internal/stage/embed_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestEmbedStage_Basic` | Valid config → embeddings in state |
| `TestEmbedStage_StateKey` | `state["document_chunks"]` is `[]types.DocumentChunk` |
| `TestEmbedStage_Count` | Output "embedding_count" matches chunk count |
| `TestEmbedStage_EmptyChunks` | No chunks → zero embeddings |
| `TestEmbedStage_ChunkEmbeddingPairing` | Each DocumentChunk links correct Chunk and Embedding by ID |

---

### 5.4 — Store Stage (`internal/stage/store.go`)

**Depends on:** 4.2 (Qdrant), Phase 1
**Parallelizable with:** 5.1, 5.2, 5.3

**Input:** `state["document_chunks"]` + connection config
**Output:** Write confirmation in state

```go
func StoreStage(cfg *config.Config) pipeline.Stage
```

**Logic:**
1. Connect to Qdrant using `cfg.QdrantURL` and `cfg.QdrantAPIKey`
2. Ensure collection exists with vector size from embedding dimension
3. Upsert all document chunks
4. Close connection
5. Return `"stored_count"` in output

**Acceptance criteria:**
- Connects to configured Qdrant URL
- Ensures collection exists
- Stores all document chunks
- Returns count of stored vectors
- Closes connection on completion

**Unit tests (`internal/stage/store_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestStoreStage_Basic` | Stage returns stored_count matching input count |
| `TestStoreStage_EmptyChunks` | Empty list → zero stored, no error |
| `TestStoreStage_StateKey` | No new state key needed (stage stores externally) |
| `TestStoreStage_CollectionName` | Collection named correctly (e.g., "document_chunks") |
| `TestStoreStage_ConnectionError` | Invalid URL → error |

**Note:** Store stage tests that need a real Qdrant server should use `t.Skip("requires Qdrant server")` with build tag `integration`, similar to Qdrant backend tests.

---

## Phase 6: CLI & Integration

### 6.1 — CLI Main (`cmd/index/main.go`)

**Depends on:** Phase 5 (all stages)
**Input:** CLI flags, env vars
**Output:** Running pipeline

```go
func main() {
    cfg, err := config.Load()
    if err != nil {
        log.Fatal(err)
    }

    idxPipeline := pipeline.Pipeline{
        Journal: journal.NewGobFileJournal(".journal-index"),
        Config:  cfg,
        Stages: []pipeline.Stage{
            stagepkg.ParseStage(cfg),
            stagepkg.ChunkStage(cfg),
            stagepkg.EmbedStage(cfg),
            stagepkg.StoreStage(cfg),
        },
    }

    ctx := context.Background()
    if err := idxPipeline.Run(ctx); err != nil {
        log.Fatal(err)
    }
}
```

**Additional flags:** `--from` (resume), all config flags from Phase 1.2

**Acceptance criteria:**
- `go build ./cmd/index` succeeds
- Running binary with valid flags runs pipeline
- `--from parse` resumes from parse stage
- `--from chunk` resumes from chunk stage
- Non-zero exit on failure
- Uses `.journal-index/` for journal (separate from preprocess)

**Unit tests (`cmd/index/main_test.go`):**

| Test | What it validates |
|------|------------------|
| `TestBuildIndexPipeline` | Pipeline wired with correct stages in order |
| `TestIndexPipeline_StageCount` | Exactly 4 stages |
| `TestIndexPipeline_StageOrder` | Stages: parse → chunk → embed → store |
| `TestIndexPipeline_JournalPath` | Uses `.journal-index/` directory |
| `TestIndexPipeline_JournalsSeparate` | Index journal does not conflict with `.journal/` |
| `TestIndexPipeline_FromFlag` | Resume works with valid stage names |
| `TestIndexPipeline_InvalidFromFlag` | Unknown stage → error |

### 6.2 — Makefile Update

**Depends on:** 6.1

**Additions to `make.cmd`:**

```
build-index:
    go build -o bin/index.exe ./cmd/index

run-index: build-index
    .\bin\index.exe

clean-index:
    Remove-Item -Recurse -Force .journal-index
```

**Acceptance criteria:**
- `make.cmd build-index` succeeds
- `make.cmd run-index` runs the indexing pipeline (requires preprocessing output)

### 6.3 — Integration Test

**Depends on:** Phase 6 (CLI)

**Test steps:**
1. Run preprocessing pipeline to produce `output/`
2. Run indexing pipeline with fixed chunker, local embedder (mock/real)
3. Assert pipeline completes without error
4. Assert journal exists with all 4 stages succeeded
5. Assert stage output contains correct counts

**For CI (no Qdrant/OpenAI available):**
- Use build tag `//go:build integration`
- Test parsing + chunking as a dry run (skip embed + store)
- Or use a `--dry-run` flag

**Acceptance criteria:**
- Indexing pipeline completes on preprocessed handbook data
- All 4 stages recorded as succeeded in journal
- Chunk counts are reasonable (> 4000 chunks for 4496 files with default settings)

---

## Testing Philosophy

Unit tests are written **within each phase** alongside the implementation, not deferred. This ensures:
- **Immediate feedback:** Bugs caught at implementation time
- **Test-first thinking:** Edge cases considered while writing code
- **Small, focused tests:** Each package has targeted coverage
- **No regression risk:** Changes validated immediately

### Testing Conventions

| Convention | Rule |
|-----------|------|
| Table-driven tests | Use `tests []struct{...}` for multiple cases |
| Temp directories | Use `t.TempDir()` — never hardcode paths |
| Mock servers | Use `httptest.NewServer` for HTTP backends |
| Build tags | Use `//go:build integration` for tests requiring external services |
| Skip message | Tests requiring external services: `t.Skip("requires <service>")` |
| Concurrent safety | Never share state between parallel tests without synchronization |

### Estimated test counts

| Package | Est. tests | Key areas |
|---------|-----------|-----------|
| `internal/types` (indexing.go) | 8 | Chunk, Embedding, DocumentChunk creation/zero-value |
| `internal/config` (additions) | 17 | Validation rules, env overrides, defaults |
| `internal/parser` | 14 | File walking, filtering, edge cases |
| `internal/chunker` (fixed) | 12 | Size/overlap, boundaries, IDs, empty |
| `internal/chunker` (semantic) | 11 | Heading split, hierarchy, no headings, nesting |
| `internal/chunker` (recursive) | 12 | Separator priority, max size, fallback |
| `internal/embedder` (openai) | 14 | OpenAI-compatible API, batching, rate limit, errors |
| `internal/store` (qdrant) | 14 | Connect, collection, upsert, point conversion |
| `internal/stage` (parse) | 4 | State passing, error, empty dir |
| `internal/stage` (chunk) | 6 | Strategy selection, empty, count |
| `internal/stage` (embed) | 5 | Pairing, empty, count |
| `internal/stage` (store) | 5 | Connection, count, empty, error |
| `cmd/index` | 7 | Pipeline wiring, resume, journal |
| **Total** | **123** | All passing via `go test ./...` |

---

## Agent Assignment Recommendations

| Agent | Work Item | Est. Effort | Dependencies |
|-------|-----------|-------------|--------------|
| A | 1.1 (types) | Small | None |
| B | 1.2 (config) | Medium | None |
| C | 2.1 (parser) | Medium | Phase 1 |
| D | 2.2 (chunker interface + fixed) | Medium | Phase 1 |
| E | 2.3 (semantic chunker) | Small | Phase 1 |
| F | 2.4 (recursive chunker) | Medium | Phase 1 |
| G | 3.1 + 3.2 (interface + openai-compatible embedder) | Medium | Phase 1 |
| H | 4.1 + 4.2 (interface + qdrant) | Medium | Phase 1 |
| I | 5.1 (parse stage) | Small | Phase 1 + 2.1 |
| J | 5.2 (chunk stage) | Small | Phase 1 + 2.2/2.3/2.4 |
| K | 5.3 (embed stage) | Small | Phase 1 + 3.2 |
| L | 5.4 (store stage) | Small | Phase 1 + 4.2 |
| M | 6.1 (CLI main.go) | Small | Phase 5 |
| N | 6.2 (make.cmd) | Tiny | 6.1 |
| O | 6.3 (integration test) | Small | Phase 6 |

**Parallelization strategy:**
- **Wave 1:** A, B (Phase 1 — types + config)
- **Wave 2:** C, D, E, F, G, H (parser, chunkers, embedder, store — all depend only on Phase 1)
- **Wave 3:** I, J, K, L (pipeline stages — each depends on one component from Wave 2)
- **Wave 4:** M, N, O (CLI, make.cmd, integration)

Total: ~15 agent tasks, completed in 4 waves with up to 6 agents in parallel in Wave 2.

---

## Planned Test File Manifest

```
internal/types/indexing_test.go        — 8 tests
internal/config/config_test.go         — +17 tests (additions to existing)
internal/parser/parser_test.go         — 14 tests
internal/chunker/fixed_test.go         — 12 tests
internal/chunker/semantic_test.go      — 11 tests
internal/chunker/recursive_test.go     — 12 tests
internal/embedder/openai_test.go       — 14 tests
internal/store/qdrant_test.go          — 14 tests
internal/stage/parse_test.go           — 4 tests
internal/stage/chunk_test.go           — 6 tests
internal/stage/embed_test.go           — 5 tests
internal/stage/store_test.go           — 5 tests
cmd/index/main_test.go                 — 7 tests
```

All tests should pass with `go test ./...` without requiring external services (Qdrant, OpenAI). Tests that require external services use build tags and clear skip messages.
