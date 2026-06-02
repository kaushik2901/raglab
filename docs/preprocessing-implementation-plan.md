# Preprocessing Pipeline — Implementation Plan

## Context Summary

| Metric                   | Value                                  |
| ------------------------ | -------------------------------------- |
| Markdown files           | 4,487                                  |
| Local shortcodes         | ~70 in `layouts/_shortcodes/`          |
| Theme shortcodes         | Unknown (Docsy + docsy-gitlab modules) |
| `{{% include %}}` usages | 296                                    |
| Raw HTML `<div>`         | 325+                                   |
| Raw HTML `<table>`       | 1,652+                                 |
| `<iframe>` embeds        | 340+                                   |
| `<style>` blocks         | 13                                     |
| `<script>` blocks        | 22                                     |

## Project Structure — Unified RAG Repo

```
root/
│
├── cmd/                          # CLI entry points (one per workflow)
│   ├── preprocess/               # Preprocessing: clone → clean → verify
│   │   └── main.go
│   ├── index/                    # (future) Indexing: parse → chunk → embed → store
│   ├── serve/                    # (future) Query/retrieval API
│   └── eval/                     # (future) Evaluation/benchmarks
│
├── internal/
│   │
│   ├── pipeline/                 # Shared stage runner (reused by all workflows)
│   │   └── pipeline.go           # Stage orchestration, journal, retry, resume, progress
│   │
│   ├── stage/                    # Stage implementations
│   │   ├── clone.go              # Preprocessing: git clone/pull
│   │   ├── preprocess.go         # Preprocessing: markdown preprocessing
│   │   ├── verify.go             # Preprocessing: output verification
│   │   ├── parse.go              # (future) document parsing
│   │   ├── chunk.go              # (future) chunking strategies
│   │   ├── embed.go              # (future) embedding generation
│   │   └── store.go              # (future) vector storage
│   │
│   ├── preprocessor/             # Markdown preprocessing logic
│   │   ├── preprocessor.go       # Orchestrates all transforms on a single file
│   │   ├── includes.go           # Resolve {{% include "path" %}}
│   │   ├── shortcodes.go         # Strip/resolve shortcode tags
│   │   ├── html.go               # Strip/convert raw HTML
│   │   └── refs.go               # Resolve {{< ref >}} / {{< relref >}}
│   │
│   ├── chunker/                  # (future) chunking strategies
│   │   ├── chunker.go            # Chunker interface
│   │   ├── fixed.go              # Fixed-size chunking
│   │   ├── semantic.go           # Semantic/section-based chunking
│   │   └── recursive.go          # Recursive character splitting
│   │
│   ├── embedder/                 # (future) embedding model abstraction
│   │   ├── embedder.go           # Embedder interface
│   │   ├── openai.go             # OpenAI-compatible API
│   │   └── local.go              # Local model (e.g., sentence-transformers)
│   │
│   ├── store/                    # (future) vector store abstraction
│   │   ├── store.go              # VectorStore interface
│   │   ├── postgres.go           # pgvector backend
│   │   └── lancedb.go            # LanceDB backend
│   │
│   ├── retriever/                # (future) retrieval strategies
│   │   ├── retriever.go
│   │   ├── simple.go             # Basic similarity search
│   │   └── hybrid.go             # Hybrid keyword + vector
│   │
│   ├── journal/                  # Stage result persistence
│   │   ├── journal.go            # Journal interface
│   │   ├── gob.go                # GobFileJournal (file-based)
│   │   └── sqlite.go             # (future) SQLite-backed journal
│   │
│   ├── config/                   # Shared configuration
│   │   └── config.go
│   │
│   └── types/                    # Shared domain types
│       ├── document.go           # Document, Chunk, Vector types
│       └── pipeline.go           # Stage, StageRecord, etc.
│
├── go.mod
├── go.sum
└── Makefile
```

## Stage Definitions

### Stage 1: Clone / Pull (internal/stage/clone.go)

**Input:** Source repo URL, target path
**Output:** Latest repo contents on disk
**Idempotency key:** "clone"

**Logic:**

- If path exists: `git fetch origin && git checkout default_branch && git pull --ff-only`
- If path does not exist: `git clone <url> <path>`

**Edge cases:**

- Network failure → retry with backoff
- Repo in detached HEAD → handle gracefully
- Credential issues → fail fast

### Stage 2: Preprocess Markdown (internal/stage/preprocess.go)

**Input:** Source path, output path
**Output:** Clean markdown files in output path
**Idempotency key:** "preprocess" + content hash of each file

**Processing order (single pass per file):**

```
for each .md file in content/:
    1. resolve {{% include "path" %}}     → inline included file content
    2. strip all other shortcodes:
       - self-closing:  {{< foo bar >}}               → remove entirely
       - paired:        {{< foo >}}...{{< /foo >}}     → keep inner content, strip tags
       - markdown-mode: {{% foo %}}...{{% /foo %}}     → same as above
    3. strip raw HTML tags, preserve inner text:
       - <div>...</div>           → keep text content
       - <table>...</table>       → flatten to text
       - <style>...</style>       → remove entirely
       - <script>...</script>     → remove entirely
       - <iframe ...>             → remove (or keep URL as reference)
       - <img ...>                → keep alt text, remove tag
       - <a href="...">text</a>   → keep text [href]
    4. resolve {{< ref "path" >}} → convert to relative markdown link
    5. write output preserving directory structure
```

**Shortcode handling strategy:**

| Category           | Examples                              | Action                             |
| ------------------ | ------------------------------------- | ---------------------------------- |
| Content includes   | include                               | **Resolve** — inline file content  |
| Content wrappers   | details, alert, panel, cardpane, card | **Strip tags, keep inner content** |
| Navigation/ToC     | handbook-data-toc, section-inline-toc | **Remove entirely**                |
| Dynamic lookups    | member-by-name, member-by-gitlab      | **Remove entirely**                |
| Media embeds       | youtube                               | **Remove entirely**                |
| References         | ref, relref                           | **Resolve** to absolute links      |
| Data tables        | engineering/support-skill-areas       | **Remove entirely**                |
| Unknown shortcodes | Any unrecognized                      | **Strip tags, keep inner content** |

**Why not render shortcodes?** Shortcodes execute Go templates with access to Hugo's .Site.Data, .Page, and API data. Rendering them statically would require reimplementing Hugo's template engine, loading all YAML/JSON data files, and hitting external APIs — defeating the purpose of avoiding Hugo.

**Include resolution:** Includes are recursive — included files may themselves contain shortcodes. Algorithm:

1. Read source file
2. Find `{{% include "path" %}}` pattern
3. Read included file (path relative to repo root)
4. Recursively process the included content
5. Replace the include tag with resolved content
6. Write to output

**Edge cases:** Circular includes (detect via visited set, break with error), missing include path (skip with warning), binary/non-markdown includes (skip).

### Stage 3: Verify Output (internal/stage/verify.go)

**Input:** Output path
**Output:** Verification report + exit code

**Checks:**

1. **File count match** — output has same number of .md files as source content/
2. **Directory structure preserved** — same relative paths
3. **No shortcodes remain** — grep for remaining patterns
4. **No raw HTML remains** — grep for remaining tags
5. **Minimum content per file** — no empty output files
6. **Total size sanity** — total bytes within reasonable range

**Output:** JSON report with pass/fail per check.

## Pipeline Runner

```go
type Stage struct {
    Name     string
    Run      func(ctx context.Context, state map[string]any) error
    Requires []string
}
```

**Features:**

- **Journaling:** GobFileJournal persists StageRecord after each stage
- **Idempotency:** On re-run, check journal — skip if already succeeded
- **Retry:** Exponential backoff for transient failures
- **Resume:** On restart, load journal, skip completed stages, re-run from first failed
- **Progress:** Simple callback for UI later
- **Linear execution** (4 stages, no DAG needed)

**Execution flow:**

```
load journal
for each stage in order:
    if journal.HasSuccess(name) && sameInputHash:
        skip (idempotent)
        continue
    run stage with retry
    save result to journal
    report progress
```

## Implementation Order

| Step | What                                                                              | Why                                   |
| ---- | --------------------------------------------------------------------------------- | ------------------------------------- |
| 1    | `go mod init` + scaffold `cmd/preprocess/`, `internal/types/`, `internal/config/` | Project skeleton                      |
| 2    | `internal/pipeline/pipeline.go` + `internal/journal/`                             | Generic runner with idempotency       |
| 3    | `internal/stage/clone.go`                                                         | Get data to process                   |
| 4    | `internal/preprocessor/includes.go`                                               | Resolve `{{% include %}}`             |
| 5    | `internal/preprocessor/shortcodes.go`                                             | Strip shortcode tags                  |
| 6    | `internal/preprocessor/html.go`                                                   | Strip/convert raw HTML                |
| 7    | `internal/preprocessor/refs.go`                                                   | Resolve `{{< ref >}}`                 |
| 8    | `internal/preprocessor/preprocessor.go`                                           | Orchestrate all transforms            |
| 9    | `internal/stage/preprocess.go`                                                    | Wire preprocessor as a pipeline stage |
| 10   | `internal/stage/verify.go`                                                        | Output verification stage             |
| 11   | Wire `cmd/preprocess/main.go`                                                     | Complete CLI flow                     |
| 12   | Test on real handbook                                                             | Iterate                               |

## Extensibility for Future RAG Pipeline

The pipeline runner is intentionally generic — stages are just `func(ctx, state) error`. When you build the RAG ingestion pipeline, you add:

```go
stages := []pipeline.Stage{
    {Name: "parse",   Run: parseDocuments},
    {Name: "chunk",   Run: chunkDocuments},
    {Name: "embed",   Run: generateEmbeddings},
    {Name: "store",   Run: storeVectors},
}
```

The same journal/retry/resume machinery works unchanged. Strategy swapping goes in the stage function body:

```go
func chunkDocuments(ctx context.Context, state map[string]any) error {
    strategy := state["chunk_strategy"].(string)
    chunker := chunkerRegistry[strategy]
    // ...
}
```
