# Streaming Architecture Design

Fixes [known issue #2](known-issues-and-fixes.md#2-no-streaming--entire-file-loaded-in-memory-before-processing)
and defines extensible interfaces for all current and future strategies.

## Design Principle

Every pipeline stage is an interface. Strategies are implementations in separate files.
Adding a new strategy means writing one new file — zero changes to existing code.

## Core Abstraction: Structured Document Elements

Instead of passing raw text (`types.Document.Content`), the pipeline passes a stream of
typed structural elements. Each element carries enough information for any chunking
strategy to make intelligent split decisions.

```go
// internal/types/element.go

// Element is a single structured unit from parsing.
// Kinds: "heading", "paragraph", "code_block", "table", "list_item", "word"
type Element struct {
    Kind  string
    Text  string
    Level int               // heading level (0 for non-headings)
    Meta  map[string]string // e.g. {"language": "go", "url": "..."}
}

// ElementReader yields elements sequentially from a document source.
type ElementReader interface {
    ReadElement() (Element, error) // io.EOF when done
    Path() string
    Close() error
}
```

This is the single data contract between every parser and every chunker.

## Pipeline Overview

```
                    Element stream                Chunk stream
  File ──► Parser ──► ElementReader ──► Chunker ──► <-chan Chunk ──► Embedder ──► Store
               ▲                        ▲                ▲
               │                        │                │
          MarkdownParser           FixedChunker     (unchanged)
          SemanticParser     SemanticChunker
                              RecursiveChunker
```

No stage ever holds the full file in memory.

## Parser Interface — `internal/parser/parser.go`

```go
// Parser parses a file into a stream of structured elements.
type Parser interface {
    Parse(filePath string) (ElementReader, error)
}
```

| Strategy       | What it produces                                                                 |
|----------------|----------------------------------------------------------------------------------|
| `markdown`     | Headings, paragraphs, code blocks, tables, list items from `.md` files           |
| `semantic`     | Same structure but with NLP-extracted concepts, entities, summaries per section  |
| `html`         | Strips tags, yields headings + text blocks from HTML                             |

A `MarkdownParser` uses `bufio.Scanner` line-by-line and regex to detect heading markers
(`#`, `##`), code fences, and list markers. It yields one `Element` per structural unit
and never holds more than a few lines in memory.

## Chunker Interface — `internal/chunker/chunker.go`

```go
// Chunker consumes an element stream and produces chunks.
type Chunker interface {
    Chunk(ctx context.Context, reader ElementReader, docPath string) (<-chan types.Chunk, <-chan error)
}
```

| Strategy       | How it splits                                                                                       |
|----------------|-----------------------------------------------------------------------------------------------------|
| `fixed`        | Reads `Text` from each element, tokenizes into words, builds fixed-size sliding windows (current behavior, but streamed) |
| `semantic`     | Uses heading/paragraph boundaries from element kinds + optional LLM boundary scoring                |
| `recursive`    | LangChain-style: tries paragraph split first, falls back to sentence, then word                     |
| `llm`          | Calls an LLM to propose chunk boundaries based on semantic completeness                             |

### `FixedChunker` algorithm (streaming)

```
window = []
for each element from reader:
    words = tokenize(element.Text)
    for each word in words:
        append word to window
        while len(window) >= Size:
            yield window[0:Size] as Chunk
            window = window[step:]   // step = Size - Overlap
// EOF: yield remaining window if any (at least one new word since last yield)
```

Memory per file: `Size` words in flight — no more.

### `SemanticChunker` algorithm (future)

```
buffer = []
for each element from reader:
    append element to buffer
    if is_chunk_boundary(buffer):  // e.g. new H1, or buffer exceeds max tokens
        yield elements_as_chunk(buffer)
        buffer = [overlap_elements]
yield remainder
```

The `SemanticChunker` never splits inside a heading or code block. It uses the
`Element.Kind` and `Element.Level` fields to make boundary decisions.

## Retriever Interface — `internal/retriever/retriever.go`

Already a strategy pattern. Extended with new strategies only.

```go
type Retriever struct {
    embedder embedder.Embedder
    store    store.VectorStore
    strategy string
}

func (r *Retriever) Retrieve(ctx context.Context, collection string, query string, topK int) ([]types.SearchResult, error)
```

| Strategy         | How it retrieves                                                                 |
|------------------|----------------------------------------------------------------------------------|
| `naive-search`   | Embed query → vector search (current)                                            |
| `hybrid`         | Embed query + extract keywords → vector search + BM25/lexical → fuse results     |
| `agentic`        | Multi-step: retrieve → LLM evaluates → reformulate query → retrieve again        |
| `multi-vector`   | Retrieve multiple query embeddings (e.g. query + hypothetical answer) → union    |

## Embedder — unchanged

The `Embedder` interface already accepts `[]types.Chunk` and returns `[]types.Embedding`.
The streaming pipeline feeds it one batch at a time.

```go
type Embedder interface {
    Embed(ctx context.Context, chunks []types.Chunk) ([]types.Embedding, error)
    Dimensions() int
    ModelName() string
}
```

No change needed.

## Index Worker — streaming loop

```go
func streamFile(ctx context.Context, fp, relPath string, chunkr Chunker, emb Embedder,
    qStore store.VectorStore, batchSize int, collectionName string) error {

    reader, err := parser.Parse(fp)     // returns ElementReader
    if err != nil {
        return fmt.Errorf("parse %s: %w", relPath, err)
    }
    defer reader.Close()

    chunkChan, errChan := chunkr.Chunk(ctx, reader, relPath)

    var batch []types.Chunk
    for {
        select {
        case chunk, ok := <-chunkChan:
            if !ok {
                // flush remainder
                if len(batch) > 0 {
                    if err := embedAndStore(ctx, emb, qStore, collectionName, batch); err != nil {
                        return err
                    }
                }
                return nil
            }
            batch = append(batch, chunk)
            if len(batch) >= batchSize {
                if err := embedAndStore(ctx, emb, qStore, collectionName, batch); err != nil {
                    return err
                }
                batch = batch[:0]
            }
        case err := <-errChan:
            return fmt.Errorf("chunk %s: %w", relPath, err)
        }
    }
}
```

**Memory per goroutine:** `batchSize` chunks (~6 MB) + chunker window (~`Size` words).

## CLI Flag Semantics

All strategies are selected via CLI flags, validated against a registry:

```powershell
.\bin\index.exe --parser markdown --chunker semantic --tag my-collection
.\bin\query.exe --retriever hybrid --llm-provider openai --tag my-collection --query "..."
```

Every strategy is a registered name in a map:

```go
// internal/parser/registry.go
var parsers = map[string]func() Parser{
    "markdown": func() Parser { return &MarkdownParser{} },
}

// internal/chunker/registry.go
var chunkers = map[string]func(size, overlap int) Chunker{
    "fixed":     func(s, o int) Chunker { return NewFixedChunker(s, o) },
    "semantic":  func(s, o int) Chunker { return NewSemanticChunker(s, o) },
}
```

New strategy = register in map + one file implementing the interface.

## File Layout

```
internal/
  types/
    element.go        ← Element, ElementReader (single data contract)
    indexing.go       ← Chunk, Embedding, DocumentChunk (unchanged)
    document.go       ← Document (removed — no longer used in index pipeline)
    query.go          ← SearchResult (unchanged)

  parser/
    parser.go         ← Parser interface + registry
    markdown.go       ← MarkdownParser
    semantic.go       ← Future: SemanticParser

  chunker/
    chunker.go        ← Chunker interface + registry
    fixed.go          ← FixedChunker (streaming, no batch compat)
    semantic.go       ← Future: SemanticChunker
    recursive.go      ← Future: RecursiveChunker

  embedder/
    embedder.go       ← Embedder interface (unchanged)
    openai.go         ← OpenAI implementation (unchanged)

  retriever/
    retriever.go      ← Retriever + strategy dispatch
    hybrid.go         ← Future: HybridRetriever
    agentic.go        ← Future: AgenticRetriever

  store/
    store.go          ← VectorStore interface (unchanged)
    qdrant.go         ← Qdrant implementation (unchanged)

  workflow/
    index_worker.go   ← Uses Chunker, Embedder, VectorStore interfaces
```

## Summary

| Concern              | Current (broken)                | New (extensible)                      |
|----------------------|---------------------------------|---------------------------------------|
| Parsing              | `io.ReadAll` → full string      | `ElementReader` → structured stream   |
| Chunking             | `strings.Fields` → all words    | Sliding window on word stream         |
| Chunker extensibility| None — hardcoded `FixedChunker` | `Chunker` interface + registry        |
| Parser extensibility | None — free functions           | `Parser` interface + registry         |
| Retriever extensibility | Basic `naive-search` only    | Strategy dispatch, add by file        |
| Memory per file      | ~80+ MB × concurrency           | ~6 MB × concurrency                   |
| Backward compat      | —                               | Not needed — clean break              |
