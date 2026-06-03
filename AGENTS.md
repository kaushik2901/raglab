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
| 2     | `preprocess` | `clone`             | Reads `{repo}/content/`, writes cleaned markdown to `--output`                       |
| 3     | `verify`     | `clone, preprocess` | Writes `_verification_report.json` to output dir                                     |

Stages are defined in `cmd/preprocess/main.go:69-72`. Use `--from <stage>` to resume.

## Quirks

- Package `internal/stage/` is named `stageimport` (not `stage`) — imports use `stagepkg "..."`.
- `handbook/` is the default clone target, NOT tracked in git (but not gitignored either).
- Journal caching lives in `.journal/` (gob files per stage). Delete it to force re-run.
- `config.Config` is system-level only: `MAX_RETRIES`, `RETRY_BACKOFF`, `LOG_LEVEL`, `DATABASE_URL`, `LLM_BASE_URL`, `LLM_API_KEY`, `QDRANT_URL`, `QDRANT_API_KEY`.
- Pipeline inputs (repo URL, chunk params, etc.) are parsed inline by each `cmd/*/main.go`, not in config. Stage functions accept explicit parameters instead of `*config.Config` — see per-stage signatures.
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
- **Config:** system-level only (`MAX_RETRIES`, `RETRY_BACKOFF`, `LOG_LEVEL`, `DATABASE_URL`, `LLM_BASE_URL`, `LLM_API_KEY`, `QDRANT_URL`, `QDRANT_API_KEY`); indexing params are parsed inline in `cmd/index/main.go`
- **Parser:** `internal/parser/parser.go` — walks output dir, reads `.md` files into `[]types.Document`
- **Fixed chunker:** `internal/chunker/` — word-window splitting with configurable size/overlap
- **Embedder:** `internal/embedder/` — interface + OpenAI-compatible HTTP embedder with batching and rate-limit retry

Only the `fixed` chunking strategy is active. Semantic and recursive chunkers will be added after the end-to-end pipeline is complete.

## Project Vision

`docs/project-vision-and-roadmap.md` captures the full vision: durable workflows, UI/API, evaluation system, observability, and a QA chatbot. It contains detailed architecture, phase plan, and open questions.

## Key Decision: River for Workflow Engine

We use **River** (`github.com/riverqueue/river`) as the job engine — a lightweight Go queue backed by Postgres, not Temporal. River provides durable at-least-once execution, retries with backoff, and concurrency control. A thin coordination layer handles linear DAG step sequencing (each worker enqueues the next step on success). Postgres is the single source of truth for workflow state; River's internal tables are secondary.

## Phase 1: River Implementation

`docs/river-implementation-plan.md` contains the detailed implementation plan for wrapping preprocessing and indexing as durable River workflows. The plan is organized into 6 sub-phases: Postgres/River infra → workflow DB layer → preprocessing workers → indexing workers → thin CLI wrappers → journal cleanup.

## Future Evaluation

`docs/` also contains plans for an evaluation system — not yet implemented.
