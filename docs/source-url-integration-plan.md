# Source URL Integration — Implementation Plan

## Problem

When the chat API answers a question, it returns `SourceDoc` with only `document_path` (e.g. `handbook/travel-policy.md`). There is no link to the live web page. Users cannot click through to verify or explore the source.

## Solution Overview

Add a `BaseURL` job parameter to preprocessing. During preprocessing, derive a `source_url` for each document, inject it into the file's YAML front matter, and write to disk. During indexing, the parser strips front matter from content and exposes it as metadata. This metadata flows through chunk → Qdrant payload → search result → API response, giving the chat endpoint a `source_url` per document.

```
Preprocess (BaseURL injected)
  → source_url in YAML front matter on disk
    → Parser strips front matter, exposes Metadata()
      → Chunker populates Chunk.Metadata["source_url"]
        → Qdrant payload includes source_url
          → SearchResult.Metadata["source_url"]
            → SourceDoc.SourceURL → client can render link
```

## Design Decisions (Resolved)

| # | Decision | Choice |
|---|---|---|
| D1 | URL calculation from document path | Strip `.md` suffix, add trailing slash. `_index.md` files → strip `/` + `_index.md`. Matches Hugo default permalink scheme. |
| D2 | Front matter injection in preprocessor | Parse existing YAML front matter if present. Inject `source_url` key. Use `gopkg.in/yaml.v3` (add direct `require`). Overwrite if key already exists. |
| D3 | Metadata threading via parser | Add `Metadata() map[string]string` to `ElementReader` interface. Parser pre-scans raw source for `---` block **before goldmark**, strips it, parses YAML, exposes via `Metadata()`. Chunker populates `Chunk.Metadata`. |
| D4 | Type representation | `SearchResult` gains generic `Metadata map[string]string`. `SourceDoc` (API) gains typed `SourceURL string`. Internal layers use generic map for extensibility. |
| D5 | Config source | `BaseURL` is a `PreprocessArgs` / `PreprocessRequest` field. No env vars. |

## Notes on Codebase State

- **Parser is goldmark-based** (`github.com/yuin/goldmark v1.8.2` with GFM extension). No custom line-by-line scanner. The entire file is read into `[]byte`, parsed to AST, elements are pre-collected into a slice. Front matter must be stripped from raw bytes **before** goldmark parses — goldmark has no front matter concept.
- **Resilience layer is in place**: `CircuitBreakerVectorStore` wraps `QdrantStore` with independent per-operation breakers (store/search/ensure). Retry with exponential backoff on gRPC connection errors. These wrap the store transparently — our metadata changes inside the inner `QdrantStore` pass through untouched.

## Files to Change

### Phase 1 — Preprocessing: Source URL Injection

#### 1. `internal/types/document.go` — Add SourceURL field

```go
type Document struct {
	Path      string
	Content   string
	Size      int64
	SourceURL string // computed from BaseURL + Path during preprocessing
}
```

#### 2. `internal/preprocessor/frontmatter.go` — NEW file

YAML front matter utilities. No Hugo dependency — pure YAML manipulation.

```go
// HasFrontMatter reports whether content starts with a YAML front matter block
// delimited by ---.
func HasFrontMatter(content string) bool

// InjectSourceURL takes raw markdown content and a source URL string.
// Returns content with source_url injected into YAML front matter.
//
// Cases:
//   - No front matter: prepends a new --- block with source_url.
//   - Has front matter: parses existing YAML, adds/overwrites source_url key,
//     re-serializes, preserves rest of content unchanged.
func InjectSourceURL(content, sourceURL string) string

// extractFrontMatter splits content into (frontMatterYAML, body, exists).
func extractFrontMatter(content string) (fm string, body string, ok bool)
```

Implementation notes:
- Add `gopkg.in/yaml.v3` to `go.mod` as direct `require` (currently transitive via test deps).
- Front matter delimiter is `---\n` on line 1 and a second `---\n` or `---\r\n`.
- Preserve existing front matter keys exactly (no reordering, no comment loss).
- For the no-front-matter case, `yaml.Marshal(map[string]string{"source_url": url})` produces clean output.
- **No schema validation** — any YAML that opens with `---` is treated as front matter.

#### 3. `internal/preprocessor/preprocessor.go` — Inject source URL

Changes to `ProcessFile`:

```go
func ProcessFile(filePath string, repoRoot string, baseURL string) (*types.Document, error)
```

Steps to add after existing transforms:

1. Compute `sourceURL`:
   ```go
   relPath, _ := filepath.Rel(repoRoot, filePath)
   relPath = filepath.ToSlash(relPath)
   pagePath := strings.TrimSuffix(relPath, ".md")
   pagePath = strings.TrimSuffix(pagePath, "_index")
   pagePath = strings.TrimSuffix(pagePath, "/index")
   sourceURL := strings.TrimRight(baseURL, "/") + "/" + pagePath
   if !strings.HasSuffix(sourceURL, "/") {
       sourceURL += "/"
   }
   ```
2. Inject into content: `content = frontmatter.InjectSourceURL(content, sourceURL)`
3. Set `doc.SourceURL = sourceURL`

Changes to `ProcessAllFiles`:

```go
func ProcessAllFiles(ctx context.Context, srcRoot string, subdirs []string, dstDir string, concurrency int, baseURL string) (int, error)
```

Pass `baseURL` to `ProcessFile` on line ~108.

#### 4. `internal/api/types.go` — Add BaseURL to PreprocessRequest

```go
type PreprocessRequest struct {
	RepoURL     string   `json:"repo_url"`
	Tag         string   `json:"tag"`
	BaseURL     string   `json:"base_url"`     // NEW
	IncludeDirs []string `json:"include_dirs,omitempty"`
}
```

Validation: `BaseURL` is **optional** (if empty, `source_url` is not injected and `Document.SourceURL` is empty string). This preserves backward compatibility.

#### 5. `internal/workflow/preprocess_worker.go` — Add BaseURL to PreprocessArgs

```go
type PreprocessArgs struct {
	Tag         string   `json:"tag"`
	RepoURL     string   `json:"repo_url"`
	BaseURL     string   `json:"base_url"`      // NEW
	IncludeDirs []string `json:"include_dirs,omitempty"`
}
```

Pass to `ProcessAllFiles` on line ~59:

```go
_, err := preprocessor.ProcessAllFiles(ctx, srcDir, args.IncludeDirs, outputPath, 10, args.BaseURL)
```

#### 6. `internal/api/handler_workflow.go` — Map field

Locate where `PreprocessRequest` is mapped to `PreprocessArgs`. Add:

```go
BaseURL: req.BaseURL,
```

Optionally update the UI form in `internal/api/index.html`.

#### 7. `internal/preprocessor/preprocessor_test.go` — Update tests

- Existing test callers of `ProcessFile` / `ProcessAllFiles` need the new `baseURL` parameter. Use `""` for tests that don't care about source URLs.
- Add test cases for:
  - File with existing front matter → `source_url` injected
  - File without front matter → front matter block created
  - File with `source_url` already present → overwritten
  - Empty `BaseURL` → no front matter changes (no-op)
  - `_index.md` → URL computed correctly (e.g., `handbook/` not `handbook/_index/`)
  - `handbook/travel-policy/index.md` → `/handbook/travel-policy/`

---

### Phase 2 — Parser: Front Matter Stripping + Metadata

#### 8. `internal/types/element.go` — Extend ElementReader interface

```go
type ElementReader interface {
	ReadElement() (Element, error)
	Path() string
	Close() error
	Metadata() map[string]string   // NEW — returns parsed front matter key-value pairs
}
```

This is a breaking interface change. Only one implementation exists (`markdownReader`), so it's safe.

#### 9. `internal/parser/markdown.go` — Strip front matter, expose metadata

The goldmark-based parser reads the entire file into `source []byte`, then feeds it to goldmark. We pre-scan the raw bytes for front matter **before** goldmark, strip it, and expose it via `Metadata()`.

Add field to `markdownReader`:

```go
type markdownReader struct {
	elems       []types.Element
	pos         int
	path        string
	frontMatter map[string]string // NEW
}
```

Modify `Parse()` — front matter is stripped from the source bytes before goldmark parses:

```go
func (p *MarkdownParser) Parse(filePath string) (types.ElementReader, error) {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("markdown parse %s: %w", filePath, err)
	}

	// NEW: extract and strip front matter before goldmark
	var fmMap map[string]string
	if body, fm, ok := splitFrontMatter(source); ok {
		fmMap = parseYAMLFrontMatter(fm)
		source = body // goldmark never sees front matter lines
	}

	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)
	elems := walkAST(doc, source)

	return &markdownReader{
		elems:       elems,
		path:        filePath,
		frontMatter: fmMap,
	}, nil
}
```

Add helper functions:

```go
// splitFrontMatter detects a --- delimited block at the start of source.
// Returns (body, fmBytes, ok) where body is source with front matter removed.
func splitFrontMatter(source []byte) (body, fm []byte, ok bool)

// parseYAMLFrontMatter parses YAML bytes into flat string map.
// Nested values are stringified via fmt.Sprintf.
// Errors return nil (warning logged).
func parseYAMLFrontMatter(data []byte) map[string]string
```

Rules for `splitFrontMatter`:
- First non-empty line must be exactly `---`.
- Scan line-by-line until a matching `---` (or `...`) delimiter.
- If either delimiter is missing or the block is >100 lines, treat as no front matter (fail-open).
- Return body = everything after the closing delimiter; fm = lines between delimiters.

Add `Metadata()` method:

```go
func (r *markdownReader) Metadata() map[string]string {
	return r.frontMatter
}
```

**Why pre-scan instead of using goldmark AST?** Goldmark does not natively handle YAML front matter. The `---` delimiter is parsed as `ast.KindThematicBreak` (thematic break / `<hr>`), which `walkAST` silently skips. The YAML content lines between the two thematic breaks would be parsed as paragraphs, leaking front matter text into the element stream. Pre-scanning is simpler and avoids a new dependency on `github.com/abhinav/goldmark-frontmatter`.

#### 10. `internal/chunker/fixed.go` — Populate Chunk.Metadata from reader

In both chunk emission sites (lines ~61-67 and ~84-90):

```go
chunk := types.Chunk{
	ID:           fmt.Sprintf("%s-chunk-%04d", docPath, idx),
	DocumentPath: docPath,
	Content:      content,
	Metadata:     reader.Metadata(),    // NEW
	TokenCount:   estimateTokens(content),
	Index:        idx,
}
```

This copies the metadata into every chunk from the document. Since all chunks from one document share the same metadata (e.g., same `source_url`), this is correct.

### Phase 3 — Store: Metadata in Qdrant Payload

#### 11. `internal/types/query.go` — Add Metadata to SearchResult

```go
type SearchResult struct {
	ChunkID      string
	DocumentPath string
	Content      string
	Score        float32
	TokenCount   int
	ChunkIndex   int
	Metadata     map[string]string   // NEW
}
```

#### 12. `internal/store/qdrant.go` — Store Chunk.Metadata in payload

Modify `toPoint()`:

```go
func toPoint(doc types.DocumentChunk) *qdrant.PointStruct {
	vectors := make([]float32, len(doc.Embedding.Vector))
	for i, v := range doc.Embedding.Vector {
		vectors[i] = float32(v)
	}

	payload := map[string]any{
		"document_path": doc.Chunk.DocumentPath,
		"content":       doc.Chunk.Content,
		"token_count":   doc.Chunk.TokenCount,
		"chunk_index":   doc.Chunk.Index,
		"model":         doc.Embedding.Model,
	}
	// Populate generic metadata from Chunk.Metadata
	for k, v := range doc.Chunk.Metadata {
		payload[k] = v
	}

	return &qdrant.PointStruct{
		Id: &qdrant.PointId{
			PointIdOptions: &qdrant.PointId_Uuid{Uuid: chunkIDToUUID(doc.Chunk.ID)},
		},
		Vectors: qdrant.NewVectors(vectors...),
		Payload: qdrant.NewValueMap(payload),
	}
}
```

Note: `source_url` from metadata is stored as a top-level payload key alongside `document_path`, `content`, etc. This avoids nesting and keeps querying simple.

#### 13. `internal/store/qdrant.go` — Read generic metadata in search

Modify `searchOnce()`:

```go
for _, p := range resp.GetResult() {
	r := types.SearchResult{
		Score: p.GetScore(),
	}
	if payload := p.GetPayload(); payload != nil {
		if v, ok := payload["content"]; ok {
			r.Content = v.GetStringValue()
		}
		if v, ok := payload["document_path"]; ok {
			r.DocumentPath = v.GetStringValue()
		}
		if v, ok := payload["token_count"]; ok {
			r.TokenCount = int(v.GetIntegerValue())
		}
		if v, ok := payload["chunk_index"]; ok {
			r.ChunkIndex = int(v.GetIntegerValue())
		}
		// Populate generic metadata from remaining payload fields
		r.Metadata = make(map[string]string)
		for k, v := range payload {
			switch k {
			case "content", "document_path", "token_count", "chunk_index", "model":
				continue // already handled or internal
			default:
				r.Metadata[k] = v.GetStringValue()
			}
		}
	}
	results = append(results, r)
}
```

This automatically picks up `source_url` and any future front matter fields.

**Resilience note:** The `CircuitBreakerVectorStore` (in `circuitbreaker.go`) wraps `QdrantStore.Store()` and `QdrantStore.Search()` transparently. Our changes to `toPoint()` and `searchOnce()` are inside the inner store. The circuit breaker delegates all payload data unchanged — no modifications needed to the breaker layer.

### Phase 4 — API: SourceDoc and Chat Context

#### 14. `internal/api/types.go` — Add SourceURL to SourceDoc

```go
type SourceDoc struct {
	DocumentPath string  `json:"document_path"`
	Score        float32 `json:"score"`
	SourceURL    string  `json:"source_url"`    // NEW
}
```

#### 15. `internal/api/service_chat.go` — Wire metadata into response

In `retrieveSources()`:

```go
sources := make([]SourceDoc, len(results))
for i, r := range results {
	sources[i] = SourceDoc{
		DocumentPath: r.DocumentPath,
		Score:        r.Score,
		SourceURL:    r.Metadata["source_url"],   // NEW
	}
}
```

In `buildMessages()`, include the URL in the LLM context:

```go
for _, r := range results {
	label := r.DocumentPath
	if url := r.Metadata["source_url"]; url != "" {
		label = url
	}
	contextParts = append(contextParts, fmt.Sprintf("Document: %s\n%s", label, r.Content))
}
```

This gives the LLM the actual URL as the document label. The LLM can cite `[source](url)` naturally.

### Phase 5 — Eval: Optional URL in Context

#### 16. `internal/eval/worker.go` — Include source_url in eval prompts

In `fillRetrievalResult` (or wherever the context prompt is built for the judge), replicate the same `Metadata["source_url"]` pattern used in the chat service.

No changes to `RelevanceJudgment` or `RetrievalResult` types — eval metrics match on `document_path` which is unchanged.

## Test Plan

| What | How |
|---|---|
| Front matter injection | Unit test `frontmatter.go`: no FM, existing FM, existing source_url, empty base URL. |
| URL derivation | Unit test: `handbook/travel-policy.md`, `_index.md`, `index.md` in subdir, top-level `about.md`. |
| Parser front matter stripping | Unit test: file with FM `---` block → `ReadElement()` produces no FM text elements, `Metadata()` returns parsed keys. File without FM → `Metadata()` returns nil. Uses `testdata/frontmatter.md`. |
| Parser backwards compat | Existing goldmark tests unchanged — files in `testdata/` have no front matter; goldmark behavior is preserved. |
| Chunker metadata propagation | Unit test: chunker output chunks have `Metadata["source_url"]` same as reader's `Metadata()` value. |
| Qdrant payload round-trip | Integration test: `toPoint`/`searchOnce` preserves `Metadata["source_url"]`. |
| API response | Unit test `retrieveSources`: `SourceDoc.SourceURL` populated from search result metadata. |
| Preprocess→index integration | End-to-end: run preprocess with BaseURL, run index with parser, verify Qdrant points contain `source_url`. |
| Circuit breaker passthrough | Existing circuit breaker tests continue to pass — metadata shape change is invisible to breaker. |

## Backward Compatibility

- **BaseURL is optional**: empty string → no front matter injection, no-op.
- **Existing front matter keys**: preserved, only `source_url` is added/overwritten.
- **Parser without front matter**: files without `---` front matter pass through goldmark as before. `Metadata()` returns nil.
- **Chunk.Metadata nil**: chunker copies `reader.Metadata()` which may be nil → `Chunk.Metadata` is nil → `toPoint` skips metadata loop.
- **Qdrant payload without source_url**: `searchOnce` skips unknown payload keys silently. `Metadata` map is empty → `SourceDoc.SourceURL` is `""`.
- **Existing collections**: not migrated. Only newly indexed documents will have `source_url`. Old collections continue to work — missing `source_url` returns empty string.
- **Circuit breaker/retry layer**: no changes needed. Metadata flows through payload fields which the breaker treats as opaque data.

## Implementation Order

| Step | File(s) | Description |
|---|---|---|
| 1 | `internal/types/document.go` | Add `SourceURL` field |
| 2 | `internal/preprocessor/frontmatter.go` | NEW — YAML front matter utilities |
| 3 | `internal/preprocessor/preprocessor.go` | Compute URL, inject front matter |
| 4 | `internal/api/types.go`, `internal/workflow/preprocess_worker.go`, handler | Add `BaseURL` to request/args, wire through |
| 5 | `internal/types/element.go`, `internal/parser/markdown.go` | Extend `ElementReader`, strip front matter pre-goldmark, expose metadata |
| 6 | `internal/chunker/fixed.go` | Populate `Chunk.Metadata` from reader |
| 7 | `internal/store/qdrant.go` | Store/retrieve metadata in Qdrant payload |
| 8 | `internal/types/query.go` | Add `Metadata` to `SearchResult` |
| 9 | `internal/api/types.go`, `internal/api/service_chat.go` | `SourceDoc.SourceURL`, context prompt |
| 10 | Tests throughout | Unit + integration tests for each phase |
