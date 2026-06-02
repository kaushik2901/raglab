# Gitlab handbook RAG pipeline

Preprocessing pipeline for GitLab's public handbook (~4,500 markdown files) that converts Hugo-based documentation into clean, LLM/RAG-friendly markdown.

## Pipeline Stages

1. **clone** — clones the handbook repo (or pulls latest if already present)
2. **preprocess** — transforms each markdown file:
   - Resolves `{{% include "path" %}}` directives (recursive with cycle detection)
   - Strips Hugo shortcodes (details, alert, panel, youtube, member-by-name, etc.)
   - Cleans raw HTML (style, script, iframe, img, a, table, div, etc.)
   - Resolves `{{< ref >}}` / `{{< relref >}}` to markdown links
3. **verify** — validates output quality (file count, directory structure, no stray shortcodes/HTML, minimum content size, total size sanity)

## Usage

```bash
# Build
go build -o bin\preprocess.exe .\cmd\preprocess

# Run (clones handbook, preprocesses, verifies)
bin\preprocess.exe

# Or use the build script
build.cmd          # build
build.cmd run      # build & run
build.cmd clean    # remove bin/ output/ .journal/
build.cmd test     # run all tests
```

### CLI Flags

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--repo-url` | `REPO_URL` | `https://gitlab.com/gitlab-com/content-sites/handbook` | Handbook repository URL |
| `--repo-path` | `REPO_PATH` | `./handbook` | Local clone path |
| `--output` | `OUTPUT_PATH` | `./output` | Cleaned markdown output directory |
| `--max-retries` | `MAX_RETRIES` | `3` | Max retries per stage on failure |
| `--retry-backoff` | `RETRY_BACKOFF` | `5s` | Initial retry backoff duration |
| `--log-level` | `LOG_LEVEL` | `info` | Log level (debug/info/warn) |
| `--from` | — | — | Resume from a specific stage name |

### Examples

```bash
# Custom output directory
bin\preprocess.exe --output .\clean-handbook

# Resume from preprocess stage
bin\preprocess.exe --from preprocess

# Fewer retries for quick testing
bin\preprocess.exe --max-retries 1 --retry-backoff 1s
```

## Testing

```bash
go test ./...
```

## Project Structure

```
cmd/preprocess/main.go          — CLI entry point
internal/
  config/config.go              — Configuration (flags + env vars)
  types/document.go             — Document type
  types/pipeline.go             — StageID, StageRecord, StageResult
  journal/journal.go            — Journal interface + gob-backed implementation
  pipeline/pipeline.go          — Generic pipeline runner (retry, cache, resume)
  preprocessor/
    includes.go                 — {{% include %}} resolver
    shortcodes.go               — Shortcode stripper with rules engine
    html.go                     — HTML cleaning
    refs.go                     — {{< ref >}} / {{< relref >}} resolver
    preprocessor.go             — Orchestrator (applies all transforms)
  stage/
    clone.go                    — Git clone/pull stage
    preprocess.go               — Preprocess pipeline stage
    verify.go                   — Verification stage
```

## License

MIT
