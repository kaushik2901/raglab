# AGENTS.md — GitLab Handbook RAG Pipeline

## Commands

```powershell
go build -o bin\preprocess.exe .\cmd\preprocess   # build
.\bin\preprocess.exe                               # run (clones handbook, preprocesses, verifies)
.\bin\preprocess.exe --from preprocess             # resume from a stage
.\bin\preprocess.exe --max-retries 1 --retry-backoff 1s  # fast dev iteration
go test ./...                                      # all tests
Remove-Item -Recurse -Force bin,output,.journal    # clean
```

On Windows, use `make.cmd` for Docker-based builds (build/run/clean/test).  
`build.cmd` does NOT exist despite README mentioning it — use `make.cmd` or raw `go build` instead.

Zero external Go dependencies (no `go.sum` yet). No `vendor/` dir.

## Pipeline Stages

| Order | Stage        | Requires            | What it does                                                                         |
| ----- | ------------ | ------------------- | ------------------------------------------------------------------------------------ |
| 1     | `clone`      | —                   | `git clone --depth 1`; if exists: `fetch --all` + `checkout main` + `pull --ff-only` |
| 2     | `sync-data`  | `clone`             | Runs `handbook/scripts/sync-data.sh` inside cloned repo (requires `sh`)              |
| 3     | `preprocess` | `clone`             | Reads `{repo}/content/`, writes cleaned markdown to `--output`                       |
| 4     | `verify`     | `clone, preprocess` | Writes `_verification_report.json` to output dir                                     |

Stages are defined in `cmd/preprocess/main.go:69-74`. Use `--from <stage>` to resume.

## Quirks

- Package `internal/stage/` is named `stageimport` (not `stage`) — imports use `stagepkg "..."`.
- `handbook/` is the default clone target, NOT tracked in git (but not gitignored either).
- Journal caching lives in `.journal/` (gob files per stage). Delete it to force re-run.
- Config reads CLI flags first, falls back to env vars: `REPO_URL`, `REPO_PATH`, `OUTPUT_PATH`, `MAX_RETRIES`, `RETRY_BACKOFF`, `LOG_LEVEL`.
- `go mod tidy` is only needed if adding external deps (currently none).
- Pipeline retries use exponential backoff + jitter. `MaxRetries=0` means no retries. `RetryBackoff` must be > 0.

## Preprocessor Transform Order

1. `ResolveIncludes` — `{{% include "path" %}}` (recursive, cycle-protected)
2. `StripShortcodes` — details/alert/panel → StripTags, youtube/handbook-data-toc/member-by-\* → Remove, include/ref/relref → Resolve
3. `ProcessHTML` — strip style/script/iframe, keep img alt text, convert `<a>` to `text [url]`, flatten tables
4. `ResolveRefs` — `{{< ref "path" >}}` / `{{< relref "path" >}}` → markdown links

## Testing

- All tests use `t.TempDir()` for temp directories.
- Flag-based tests must call `resetFlags()` (sets `flag.NewFlagSet`) before each test to avoid global state conflicts.
- Preprocessor tests write files to temp dirs then run `ProcessFile` / `ProcessAllFiles`.

## Indexing Pipeline (In Progress)

The indexing pipeline (`cmd/index/`) builds on the preprocessing output. Currently implemented:

- **Types:** `internal/types/indexing.go` — `Chunk`, `Embedding`, `DocumentChunk`
- **Config:** `internal/config/config.go` — indexing fields (`--chunk-strategy`, `--chunk-size`, etc.)
- **Parser:** `internal/parser/parser.go` — walks output dir, reads `.md` files into `[]types.Document`
- **Fixed chunker:** `internal/chunker/` — word-window splitting with configurable size/overlap
- **Embedder:** `internal/embedder/` — interface + OpenAI-compatible HTTP embedder with batching and rate-limit retry

Only the `fixed` chunking strategy is active. Semantic and recursive chunkers will be added after the end-to-end pipeline is complete.

## Future Evaluation

`docs/` also contains plans for an evaluation system — not yet implemented.
