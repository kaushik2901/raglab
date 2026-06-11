# Goldmark Migration Plan

Replace the custom hand-rolled `MarkdownParser` with `github.com/yuin/goldmark` for spec-compliant, battle-tested markdown parsing.

---

## Why

The current parser at `internal/parser/markdown.go` uses line-by-line heuristics (`bufio.Scanner` + `strings.HasPrefix`). It has blind spots:

| Missed feature           | Impact                                      |
| ------------------------ | ------------------------------------------- |
| Indented code blocks     | Code blocks without fences sink into paragraphs |
| Lists (`-`, `*`, `1.`)   | Flattened into paragraphs, no structure     |
| Blockquotes (`>`)        | Treated as paragraph text                   |
| Thematic breaks (`---`)  | Invisible to parser, may merge sections     |
| GFM tables               | Heuristic (`|...|`), not spec-compliant     |
| Inline formatting        | Raw markdown leaks into chunk text          |

Goldmark covers all of these, is widely adopted, and is actively maintained.

---

## What changes

### Files modified

| File                     | Change                                       |
| ------------------------ | -------------------------------------------- |
| `internal/parser/markdown.go` | Complete rewrite behind same interface   |
| `internal/parser/markdown_test.go` | Update 2 tests (lists + tables)    |
| `go.mod` / `go.sum`      | `go get github.com/yuin/goldmark`            |

### Files NOT modified

- `internal/types/element.go` — `Element`, `ElementReader` unchanged
- `internal/parser/parser.go` — `Parser` interface, `Default`, `RegisterParser` unchanged
- `internal/chunker/fixed.go` — only reads `elem.Text`, unaffected
- All workflow/index/api code — no changes needed

---

## Relationship to resilience plan

Zero intersection. The resilience plan (`docs/resilience-implementation-plan.md`) touches only `internal/embedder/`, `internal/generator/`, and `internal/store/` — retry decorators, circuit breakers, and Qdrant backoff. The parser is in `internal/parser/` and has no dependencies on any of those packages. Both plans can be implemented independently in any order.

---

## Implementation

### Step 1: Add dependency

```powershell
go get github.com/yuin/goldmark
```

### Step 2: Rewrite `markdown.go`

#### Parse flow

```
os.ReadFile(filePath)
    ↓
goldmark.DefaultParser().Parse(reader) → ast.Document
    ↓
ast.Walk(doc, visitor) → []types.Element
    ↓
markdownReader{elems, pos, path}
```

#### AST → Element mapping

| goldmark node                 | Element Kind    | Level | Text source                              | Meta             |
| ----------------------------- | --------------- | ----- | ---------------------------------------- | ---------------- |
| `*ast.Heading`                | `heading`       | `h.Level` | walk inline children, join rendered text | nil              |
| `*ast.Paragraph`              | `paragraph`     | 0     | walk inline children, join rendered text | nil              |
| `*ast.FencedCodeBlock`        | `code_block`    | 0     | `n.Lines()` → join                       | `{language: …}`  |
| `*ast.CodeBlock` (indented)   | `code_block`    | 0     | `n.Lines()` → join                       | nil              |
| `*ast.Table`                  | `table`         | 0     | walk cells row-by-row, join with spaces  | nil              |
| `*ast.ListItem`               | `list_item`     | 0     | walk children, join text                 | nil              |
| `*ast.Blockquote`             | `paragraph`     | 0     | walk inline children, join rendered text | nil              |
| `*ast.ThematicBreak`          | *(skip)*        | —     | —                                        | —                |

#### Inline formatting

Goldmark resolves inline nodes to rendered text:
- `**bold**` → `"bold"`
- `` `code` `` → `"code"`
- `[text](url)` → `"text"`

This produces cleaner input for embeddings than raw markdown syntax.

#### markdownReader

```go
type markdownReader struct {
    elems []types.Element
    pos   int
    path  string
}
```

`ReadElement()` indexes through the pre-collected slice — simple and safe.

### Step 3: Update tests

#### `TestMarkdownParser_Lists` (file: `lists.md`)

**Before** (custom parser): 2 paragraphs
```go
assert.Len(t, elems, 2)
assert.Equal(t, types.ElementParagraph, elems[0].Kind) // "- item1\n- item2"
assert.Equal(t, types.ElementParagraph, elems[1].Kind) // "Para"
```

**After** (goldmark): 3 elements — 2 list items + 1 paragraph
```go
assert.Len(t, elems, 3)
assert.Equal(t, types.ElementListItem, elems[0].Kind)
assert.Equal(t, "item1", elems[0].Text)
assert.Equal(t, types.ElementListItem, elems[1].Kind)
assert.Equal(t, "item2", elems[1].Text)
assert.Equal(t, types.ElementParagraph, elems[2].Kind)
assert.Equal(t, "Para", elems[2].Text)
```

#### `TestMarkdownParser_Tables` (file: `tables.md`)

**Before** (custom parser): raw markdown table rows
```go
assert.Equal(t, types.ElementTable, elems[0].Kind)
assert.Contains(t, elems[0].Text, "| A | B |")
assert.Contains(t, elems[0].Text, "|---|")
assert.Contains(t, elems[0].Text, "| 1 | 2 |")
```

**After** (goldmark): rendered cell text
```go
assert.Equal(t, types.ElementTable, elems[0].Kind)
assert.Contains(t, elems[0].Text, "A B")
assert.Contains(t, elems[0].Text, "1 2")
```

#### All other tests

No changes expected. The test data files don't contain inline formatting, so headings, code blocks, paragraphs, unicode, empty, whitespace, and mixed-content tests all pass as-is.

### Step 4: Verify

```powershell
go test ./internal/parser/...
go test ./internal/chunker/...
go test ./...
```

---

## Performance considerations

| Concern                  | Analysis                                                              |
| ------------------------ | --------------------------------------------------------------------- |
| Memory (file loading)    | Handbook files are 5–50 KB. AST overhead ~2–5x. Negligible.          |
| Memory (preprocessor)    | Preprocessor already calls `os.ReadFile` — no streaming lost.         |
| Throughput               | 10 concurrent files × 50 KB each ≈ 500 KB peak. Microseconds per parse. |
| Bottleneck               | Embedding API calls (500ms–2s each), not parsing.                     |

**Conclusion**: No measurable impact at handbook scale.

---

## Risks & mitigations

| Risk                                       | Mitigation                                                |
| ------------------------------------------ | --------------------------------------------------------- |
| Rendered inline text shifts embeddings     | Run eval comparison on baseline dataset; verify metrics   |
| Table text format changes                  | Tables are rare in handbooks; monitor eval metrics        |
| Hugo `{.class}` attribute leaks into text  | No worse than current parser; can strip in a follow-up   |
| Goldmark API changes in future releases    | Pinned in go.mod; upgrade deliberately                   |
