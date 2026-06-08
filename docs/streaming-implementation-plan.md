# Streaming Architecture — Implementation Plan

7 phases, ordered by dependency. Each phase is independently testable and mergable.

---

## Phase 0: Scaffold `Element` and `ElementReader`

**Files to create:**
- `internal/types/element.go`

**What it contains:**

```go
package types

import "io"

type Element struct {
    Kind  string
    Text  string
    Level int
    Meta  map[string]string
}

type ElementReader interface {
    ReadElement() (Element, error) // io.EOF when done
    Path() string
    Close() error
}

// Element kinds
const (
    ElementHeading   = "heading"
    ElementParagraph = "paragraph"
    ElementCodeBlock = "code_block"
    ElementTable     = "table"
    ElementListItem  = "list_item"
)
```

**Tests (`element_test.go`):**

```go
func TestElementCreation(t *testing.T) {
    // table-driven: test each kind with various Level/Meta combos
    // verify zero values are sensible
}
```

**Verify:** `go test ./internal/types/`

---

## Phase 1: `MarkdownParser` — streaming markdown reader

**Files:**
- `internal/parser/parser.go` — `Parser` interface, registry
- `internal/parser/markdown.go` — `MarkdownParser` implementation

### Interface

```go
type Parser interface {
    Parse(filePath string) (types.ElementReader, error)
}
```

### `MarkdownParser` design

Line-based scanner using `bufio.Scanner`. State machine with these states:
- `stateNormal` — reading paragraph text, accumulating lines
- `stateCodeBlock` — inside triple-backtick fences
- `stateTable` — inside markdown table rows

On each line:
1. If fence ` ``` ` → toggle `stateCodeBlock`; if exiting, flush code block element
2. If heading `^#{1,6} ` → flush pending paragraph, yield heading element
3. If blank line → flush pending paragraph
4. If table row `^\|.+\|` → accumulate into table buffer
5. Otherwise → append to paragraph buffer

On EOF → flush all remaining buffers as elements.

**Memory:** `bufio.Scanner` default buffer (64 KB) + accumulated paragraph text (until next heading/blank line). Worst case is a paragraph with no blank lines for the entire file — that's still bounded by reader buffer if we yield paragraphs greedily.

**Important design choice:** Paragraphs are yielded on blank lines or headings, NOT accumulated indefinitely. This keeps memory bounded and means the chunker can start processing earlier.

### Pseudo-implementation

```go
type MarkdownParser struct{}

func (p *MarkdownParser) Parse(filePath string) (types.ElementReader, error) {
    f, err := os.Open(filePath)
    if err != nil {
        return nil, fmt.Errorf("markdown parse %s: %w", filePath, err)
    }
    return newMarkdownReader(f, filePath), nil
}

type markdownReader struct {
    scanner *bufio.Scanner
    path    string
    buf     strings.Builder   // accumulated paragraph/table/code text
    kind    string            // current buffer kind
    inCode  bool
    inTable bool
    level   int               // current heading level (0 = not heading)
}

func (r *markdownReader) ReadElement() (types.Element, error) {
    for r.scanner.Scan() {
        line := r.scanner.Text()
        // ... state machine logic ...
        // If a complete element is ready, flush buffer and return it
    }
    // EOF: flush remaining buffer
    if r.buf.Len() > 0 {
        return r.flushElement(), nil
    }
    return types.Element{}, io.EOF
}
```

### Tests (`internal/parser/markdown_test.go`)

Strategy: parse real markdown, collect all elements into a slice, compare against expected.

```
testdata/
  simple.md          → "# Title\n\nHello world.\n\n## Sub\n\nDetails."
  headings.md        → "# H1\n## H2\n### H3\nText\n#### H4"
  codeblocks.md      → "```go\nfmt.Println()\n```\n\nPara"
  tables.md          → "| A | B |\n|---|---|\n| 1 | 2 |"
  lists.md           → "- item1\n- item2\n\nPara"
  mixed.md           → full document with all features
  empty.md           → ""
  whitespace.md      → "   \n\n  \n"
  nometadata.md      → "Just a paragraph.\n\nAnother one."
  unicode.md         → "## Héllo\n\nWörld"
```

Tests:

```
TestMarkdownParser_Headings
    Parse headings.md → verify yields [H1, H2, H3, para "Text", H4]

TestMarkdownParser_CodeBlocks
    Parse codeblocks.md → verify yields [code_block("fmt.Println()"), para "Para"]
    Verify code_block element has Meta["language"] = "go"

TestMarkdownParser_Tables
    Parse tables.md → verify table element has full pipe-delimited text

TestMarkdownParser_Paragraphs
    Parse simple.md → verify yields [H1, para "Hello world.", H2, para "Details."]

TestMarkdownParser_EmptyFile
    Parse empty.md → ReadElement returns io.EOF immediately

TestMarkdownParser_WhitespaceOnly
    Parse whitespace.md → ReadElement returns io.EOF immediately

TestMarkdownParser_BoundedMemory
    Create a file with 1M paragraphs separated by blank lines
    Verify that at most MaxParagraphBuffer bytes are held at any time

TestMarkdownParser_Error
    Parse nonexistent file → error from Parse()
```

---

## Phase 2: `FixedChunker` — streaming word-window chunker

**Files:**
- `internal/chunker/chunker.go` — `Chunker` interface, registry
- `internal/chunker/fixed.go` — `FixedChunker` (streaming only, no batch compat)

### Interface

```go
type Chunker interface {
    Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error)
}
```

### `FixedChunker` algorithm

```
window  = []string{}         // sliding window of words
step    = Size - Overlap
totalWords = 0

for each element from reader:
    words = strings.Fields(element.Text)
    for _, word := range words:
        window = append(window, word)
        totalWords++

        // drain when window is full
        while len(window) >= Size:
            yield Chunk{Content: join(window[0:Size])}
            window = window[step:]  // slide: keep overlap words
            // don't advance further — loop will add more words

// EOF: after all elements consumed, yield remaining words
if len(window) > 0 {
    yield Chunk{Content: join(window)}
}
```

Chunk IDs follow the same scheme: `"{docPath}-chunk-{index:04d}"`.
Token count estimate: same `len(Content) / 4` heuristic.

Channel behavior:
- Chunk channel is closed after the last chunk is sent
- Error channel receives at most one error (fatal), then both channels close
- Context cancellation causes immediate return (no partial yield)

### Tests (`internal/chunker/fixed_test.go`)

Strategy: construct `ElementReader` from known element sequences (test helpers), feed to `FixedChunker`, collect all chunks, assert shape and content.

```go
// Test helpers
func elementReaderFromText(path, text string) types.ElementReader {
    // wraps text as a single paragraph Element
}

func elementReaderFromElements(path string, elems ...types.Element) types.ElementReader {
    // creates in-memory reader over provided elements
}
```

Tests mirror the existing `fixed_test.go` but use the streaming interface:

```
TestFixedChunker_Basic
    Elements: [para(100 words)]  size=30, overlap=10
    Verify: chunk count matches ceil((100-10)/20) = 5
    Verify: union of all chunk word counts >= 100

TestFixedChunker_NoOverlap
    Elements: [para(100 words)]  size=30, overlap=0
    Verify: each chunk ≤ 30 words
    Verify: no overlap (no word appears in two chunks)

TestFixedChunker_EmptyDoc
    Elements: []  size=10
    Verify: no chunks emitted

TestFixedChunker_ShortDoc
    Elements: [para(3 words)]  size=100
    Verify: exactly 1 chunk with 3 words

TestFixedChunker_SingleWord
    Elements: [para("hello")]  size=10
    Verify: 1 chunk with Content "hello"

TestFixedChunker_WhitespaceOnly
    Elements: [para("   \n  \t  ")]  size=10
    Verify: no chunks emitted (strings.Fields returns empty)

TestFixedChunker_ChunkIDs
    Elements: [para(80 words)]  docPath="docs/page.md", size=30, overlap=5
    Verify: chunks[0].ID == "docs/page.md-chunk-0000"
    Verify: chunks[1].ID == "docs/page.md-chunk-0001"

TestFixedChunker_DocumentPath
    Verify: all chunks have DocumentPath == reader.Path()

TestFixedChunker_TokenCount
    Verify: each chunk.TokenCount == len(chunk.Content) / 4

TestFixedChunker_MetadataNil
    Verify: each chunk.Metadata is nil

TestFixedChunker_MultipleElements
    Elements: [para("A B C D E"), para("F G H I J")]  size=5, overlap=0
    Verify: chunk(0) has "A B C D E", chunk(1) has "F G H I J"

TestFixedChunker_ElementBoundaryCrossing
    Elements: [para("A B C"), para("D E F G H I")]  size=5, overlap=2
    Verify: chunk(0) has "A B C D E" (crosses element boundary)
    Verify: chunk(1) has "E F G H I" (starts with overlap)
    // This test proves streaming works correctly across element boundaries

TestFixedChunker_ContextCancellation
    Long text stream, cancel context after reading first chunk
    Verify: ChunkStream returns immediately, channel closes

TestFixedChunker_ExactMatchWithOldImpl
    // CRITICAL: prove streaming produces same output as old batch Chunk()
    docContent = words(107)  // non-aligned length
    size=30, overlap=10

    // Old path: Document{Content: docContent} → Chunk() → []Chunk
    // New path: ElementReader over para(docContent) → ChunkStream() → []Chunk

    Collect both, assert chunk count, IDs, content match exactly
```

**Most important test:** `TestFixedChunker_ExactMatchWithOldImpl`. This proves the streaming refactor preserves chunking behavior identically. Keep the batch `Chunk()` implementation in the test file as the reference oracle, even though it won't be in production code.

---

## Phase 3: Stream index worker

**Files to modify:**
- `internal/workflow/index_worker.go`

### What changes

The `RunIndexing` function gets a new inner loop per file:

```go
func processFile(ctx context.Context, fp, relPath, collectionName string,
    chunkr chunker.Chunker, emb embedder.Embedder, qStore store.VectorStore,
    batchSize int) (int, error) {

    reader, err := parser.Default.Parse(fp)
    if err != nil {
        return 0, fmt.Errorf("parse: %w", err)
    }
    defer reader.Close()

    chunkChan, errChan := chunkr.Chunk(ctx, reader, relPath)

    var (
        batch    []types.Chunk
        chunksCount int
    )
    for {
        select {
        case chunk, ok := <-chunkChan:
            if !ok {
                // Flush remainder
                if len(batch) > 0 {
                    if err := embedAndStore(ctx, emb, qStore, collectionName, batch); err != nil {
                        return chunksCount, err
                    }
                }
                return chunksCount, nil
            }
            batch = append(batch, chunk)
            if len(batch) >= batchSize {
                if err := embedAndStore(ctx, emb, qStore, collectionName, batch); err != nil {
                    return chunksCount, err
                }
                chunksCount += len(batch)
                batch = batch[:0]
            }
        case err := <-errChan:
            return chunksCount, fmt.Errorf("chunk: %w", err)
        }
    }
}
```

The errgroup loop becomes:

```go
for _, filePath := range mdFiles {
    fp := filePath
    g.Go(func() error {
        docCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
        defer cancel()

        relPath, err := filepath.Rel(inputDir, fp)
        if err != nil {
            return fmt.Errorf("relative path: %w", err)
        }
        relPath = filepath.ToSlash(relPath)

        chunkCount, err := processFile(docCtx, fp, relPath, collectionName, chunkr, emb, qStore, args.BatchSize)
        if err != nil {
            // graceful skip like parse/chunk errors
            mu.Lock()
            skipErrors = append(skipErrors, relPath+": "+err.Error())
            mu.Unlock()
            return nil
        }

        totalDocs.Add(1)
        totalChunks.Add(int32(chunkCount))
        slog.Info("indexed document", "path", relPath, "chunks", chunkCount)
        return nil
    })
}
```

Removed from `index_worker.go`:
- `parser.ParseFile` import and call
- `chunkr.Chunk(doc)` call
- Entire `types.Document` construction path
- `io.ReadAll` — no longer happens anywhere in indexing

### Tests

The index worker tests are integration-level (need Qdrant, Postgres). For unit coverage,
focus on `processFile` with a mock `Chunker` and mock `Embedder`:

```go
type mockChunker struct {
    chunks []types.Chunk
}

func (m *mockChunker) Chunk(ctx context.Context, reader types.ElementReader, docPath string) (<-chan types.Chunk, <-chan error) {
    ch := make(chan types.Chunk)
    errCh := make(chan error, 1)
    go func() {
        defer close(ch)
        defer close(errCh)
        for _, c := range m.chunks {
            select {
            case ch <- c:
            case <-ctx.Done():
                errCh <- ctx.Err()
                return
            }
        }
    }()
    return ch, errCh
}
```

```
TestProcessFile_EmitsAndStores
    mockChunker yields 25 chunks, batchSize=10
    Verify: embedAndStore called 3 times (10+10+5)
    Verify: returned count = 25

TestProcessFile_ContextCancel
    Cancel context mid-stream
    Verify: returns error immediately
```

---

## Phase 4: Strategy registry + CLI wiring

**Files:**
- `internal/parser/parser.go` — add `Default` variable + `New(strategy string) (Parser, error)`
- `internal/chunker/chunker.go` — add `New(strategy string, size, overlap int) (Chunker, error)`
- Modify `cmd/index/main.go` — add `--parser`, `--chunker` flags

### Registry pattern

```go
// internal/parser/parser.go
var Default Parser = &MarkdownParser{}

var parsers = map[string]func() Parser{
    "markdown": func() Parser { return &MarkdownParser{} },
}

func New(strategy string) (Parser, error) {
    fn, ok := parsers[strategy]
    if !ok {
        return nil, fmt.Errorf("unknown parser %q", strategy)
    }
    return fn(), nil
}
```

Same for chunker:

```go
// internal/chunker/chunker.go
var chunkers = map[string]func(size, overlap int) Chunker{
    "fixed": func(size, overlap int) Chunker { return NewFixedChunker(size, overlap) },
}

func New(strategy string, size, overlap int) (Chunker, error) {
    fn, ok := chunkers[strategy]
    if !ok {
        return nil, fmt.Errorf("unknown chunker %q", strategy)
    }
    return fn(size, overlap), nil
}
```

### CLI flags (in `cmd/index/main.go`)

```go
flag.StringVar(&parserStrategy, "parser", "markdown", "Parser strategy (markdown)")
flag.StringVar(&chunkerStrategy, "chunker", "fixed", "Chunker strategy (fixed, semantic, recursive)")
```

These replace the hardcoded `chunker.NewFixedChunker(...)` call.

### Test

```
TestParserRegistry_Valid
    New("markdown") → MarkdownParser, no error

TestParserRegistry_Invalid
    New("nonexistent") → error

TestChunkerRegistry_Valid
    New("fixed", 100, 20) → FixedChunker with correct Size/Overlap

TestChunkerRegistry_Invalid
    New("nonexistent", 100, 20) → error
```

---

## Phase 5: Retriever strategy dispatch

**Files to modify:**
- `internal/retriever/retriever.go`
- `cmd/query/main.go`
- `cmd/eval/main.go`

### Current state

Retriever already has a strategy pattern with `StrategyNaiveSearch`. The switch in `Retrieve()` dispatches by string.

### Extend

Make the strategy map extensible:

```go
type RetrievalFunc func(ctx context.Context, coll, query string, topK int) ([]types.SearchResult, error)

var strategies = map[string]func(*Retriever) RetrievalFunc{
    StrategyNaiveSearch: func(r *Retriever) RetrievalFunc { return r.naiveSearch },
}

func (r *Retriever) Retrieve(ctx context.Context, collection, query string, topK int) ([]types.SearchResult, error) {
    fn, ok := strategies[r.strategy]
    if !ok {
        return nil, fmt.Errorf("strategy %q not implemented", r.strategy)
    }
    return fn(r)(ctx, collection, query, topK)
}
```

New strategies register themselves via `init()` or explicit registration call.
The `validStrategies` map becomes the `strategies` map.

### Tests

```
TestRetriever_ValidStrategies
    Verify naive-search works end-to-end with mock store

TestRetriever_InvalidStrategy
    Verify error for unknown strategy
```

---

## Phase 6: Future strategy skeletons + `init()` registration

**Files to create (empty skeletons with doc comments):**
- `internal/parser/semantic.go` — `// SemanticParser produces structured elements with NLP enrichment`
- `internal/chunker/semantic.go` — `// SemanticChunker splits on heading/paragraph boundaries`
- `internal/chunker/recursive.go` — `// RecursiveChunker splits with fallback separators`
- `internal/retriever/hybrid.go` — `// HybridRetriever fuses vector + keyword results`
- `internal/retriever/agentic.go` — `// AgenticRetriever uses multi-turn LLM refinement`

Each skeleton registers itself via `init()`:

```go
// internal/chunker/semantic.go
func init() {
    RegisterChunker("semantic", func(size, overlap int) Chunker {
        return NewSemanticChunker(size, overlap)
    })
}
```

### Tests

```
TestFutureStrategies_Registered
    Verify all strategy names are registered in their respective maps
    // Ensures no import cycle issues and strategies are visible
```

---

## Phase 7: Remove dead code

**Files to remove/modify:**
- `internal/types/document.go` — delete the file (no production code uses `types.Document` anymore)
- `internal/parser/parser.go` — remove `ParseFile` and `ParseDir` free functions (they're replaced by `MarkdownParser`)

**Note:** The preprocessor still uses `types.Document`. If the preprocessor will be migrated later, document.go stays. If the preprocessor is being migrated now as part of this plan, document.go goes. Decision needed.

---

## Testing strategy summary

| Phase | What's tested | How | Key edge cases |
|-------|--------------|-----|----------------|
| 0 | `Element` creation, field defaults | Table-driven unit | Zero values, unicode |
| 1 | `MarkdownParser` element stream | Parse real `.md` files, assert element sequence | Empty, whitespace, unicode, all heading levels, code with mixed fences, nested lists |
| 2 | `FixedChunker` streaming | ElementReader constructed from text, collect chunks from channel | Empty, single word, exact multiples, element boundary crossing, context cancellation, **identical output to old batch impl** |
| 3 | `processFile` worker | Mock chunker + mock embedder | Batch alignment (N < batch, N = batch, N > batch), mid-stream cancel |
| 4 | Registry lookups | Direct calls to `New()` | Valid name, invalid name, name collisions |
| 5 | Retriever dispatch | Mock store | Valid/invalid strategy string |
| 6 | Skeleton registration | `init()` side effects | All future names resolve |

---

## Dependency graph

```
Phase 0 (Element) ─┬─► Phase 1 (Parser) ──► Phase 3 (IndexWorker)
                    │
                    └─► Phase 2 (Chunker) ──┘
                                        │
                                        └──► Phase 4 (Registry) ──► Phase 6 (Skeletons)
                                        │
                                        └──► Phase 5 (Retriever)

Phase 7 (Cleanup) ──► depends on Phase 3 being stable
```

Phases 0-2 can be done in parallel. Phase 3 needs both 1 and 2. Phases 4-5 are independent.
Phase 6 needs 4 and 5. Phase 7 is last.
