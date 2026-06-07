# Implementation Plan: API Consistency Remediation

> Based on: `docs/api-consistency-analysis.md`
> Created: 2026-06-07

## Phase Overview

| Phase | Focus | Items | Est. Effort | Priority |
|-------|-------|-------|-------------|----------|
| 1 | CLI Flag Completeness | A1, A2 | 1 day | 🔴 High |
| 2 | Dead & Unused Fields | B1, B2, B3 | 1 day | 🟡 Medium |
| 3 | Env Var Documentation | C1–C5 | 1 day | 🟡 Medium |
| 4 | Database Persistence Gaps | D1, D2 | 1-2 days | 🟡 Medium |
| 5 | Cross-CLI Consistency | E1, E2 | 1 day | 🟢 Low |
| 6 | Docker-Compose Cleanup | F1, F2 | 1 day | 🟢 Low |

**Total estimated effort:** 6-9 days

---

## Phase 1: CLI Flag Completeness

### A1. Eval CLI: Missing `--batch-size` Flag

**Files:** `cmd/eval/main.go`, `internal/workflow/eval_worker.go`

**Problem:** `EvalArgs.BatchSize` exists on the struct but has no corresponding CLI flag in `cmd/eval/main.go` — it's always zero, forcing a fallback to 20. The `index` CLI has this flag; `eval` should too.

**Implementation:**
1. Add `--batch-size` flag to `cmd/eval/main.go` with env `BATCH_SIZE`, default `20`
2. Wire it into the `EvalArgs` construction at the `riverClient.Insert` call
3. The worker fallback (`if batchSize <= 0 { batchSize = 20 }`) stays as defensive guard

**Verification:**
- `go build ./cmd/eval/`
- Run eval with explicit `--batch-size 10` and confirm no fallback

### A2. Eval CLI: Wire `--eval-concurrency` or Remove It

**Files:** `cmd/eval/main.go`, `internal/eval/pipeline.go`

**Problem:** `--eval-concurrency` flag exists, is stored in `EvalArgs.Concurrency`, but `PipelineArgs` has no `Concurrency` field and the pipeline is inherently sequential. The flag is dead cargo.

**Implementation (option A — keep, wire, but document no-op):**
1. Add `Concurrency` to `PipelineArgs` in `internal/eval/pipeline.go`
2. In `Evaluate`, log it at `Debug` but don't change sequential behavior (the doc states eval is sequential by design — rate limit is the bottleneck)
3. Document that the flag exists for future use but currently eval runs sequentially

**Implementation (option B — remove):**
1. Remove `--eval-concurrency` flag from `cmd/eval/main.go`
2. Remove `Concurrency` field from `EvalArgs`
3. Remove it from the workflow metadata map
4. Simpler but breaks backward compatibility for anyone using the flag

**Recommendation:** Option A — keep the field wired through and document it.

**Verification:**
- `go build ./...`
- Confirm flag is accepted but eval remains sequential

---

## Phase 2: Dead & Unused Fields

### B1. `EvalQuestion.Category` and `Difficulty`

**Files:** `internal/types/eval.go`

**Problem:** Both fields are parsed from dataset JSON but never consumed by any evaluation logic. Stratified analysis by category/difficulty is not possible.

**Implementation:**
1. Add `Category` and `Difficulty` passthrough from `EvalQuestion` → `RetrievalResult` in both `fillRetrievalResult` (`pipeline.go`) and `evaluateOne` (`retrieval.go`)
2. Wire them into the `eval_queries` INSERT in `store.go` (add columns via new migration or use JSONB)
3. This enables future stratified metric computation without a schema change later

**Alternative (minimal):** Leave as-is; they're harmless dead fields. Only implement if stratified analysis is planned.

**Verification:**
- `go build ./...`
- `go test ./internal/eval/`

### B2. `CloneArgs.RepoPath` Redundant Recomputation

**Files:** `internal/workflow/preprocess_worker.go`

**Problem:** `CloneWorker.Work()` computes RepoPath as `path.Join("artifacts", "preprocessing", tag, "repo")` and passes it. `PreprocessWorker.Work()` recomputes the same path. `VerifyWorker.Work()` recomputes it again. All three produce identical results.

**Implementation:**
1. Compute `repoPath` and `outputPath` once in `CloneWorker`, pass them through `PreprocessArgs` and `VerifyArgs`
2. Remove duplicate computation from `PreprocessWorker` and `VerifyWorker`
3. Use the values from `job.Args.RepoPath` / `job.Args.OutputPath` instead

**Verification:**
- `go build ./...`
- `go test ./internal/workflow/`

### B3. Docker-Compose RPM Env Vars

**Files:** `docker-compose.yml`

**Problem:** `RPM`, `OPENAI_RPM`, `GEMINI_RPM`, `OPENROUTER_RPM`, `LMSTUDIO_RPM` are declared but never consumed by Go code.

**Implementation:**
1. Either add rate-limit reading logic to `internal/config/config.go` with reasonable defaults
2. Or remove the dead vars from `docker-compose.yml`
3. If kept, document them in `README.md` env var table

**Verification:**
- `docker compose config` validates after cleanup

---

## Phase 3: Env Var Documentation

### C1. `EVAL_CONCURRENCY` — Missing From All Docs

**Files:** `README.md`, `AGENTS.md`

**Problem:** `EVAL_CONCURRENCY` env var exists in code but appears nowhere in any documentation.

**Implementation:**
1. Add to `README.md` main env var table
2. Add to `AGENTS.md` CLI flags table for eval
3. Note it's a no-op (sequential eval) or future use

### C2. `LLM_BASE_URL` / `LLM_API_KEY` — Not First-Class in README

**Files:** `README.md`

**Problem:** These appear only as fallbacks in the provider quick reference, not as standalone entries in the main env var table.

**Implementation:**
1. Add `LLM_BASE_URL` with default `https://api.openai.com` and description "LLM API base URL (overridable per-provider)"
2. Add `LLM_API_KEY` with default `""` and description "LLM API key (overridable per-provider)"

### C3. `REPO_URL` Default Mismatch

**Files:** `README.md`

**Problem:** README says `https://gitlab.com/gitlab-com/content-sites/handbook` but code default has `.git` suffix.

**Implementation:**
1. Add `.git` suffix to the README default

### C4. `LLM_MODEL` — Missing From Main Env Var Table

**Files:** `README.md`

**Problem:** Documented in CLI flag tables but not in the main env var table.

**Implementation:**
1. Add `LLM_MODEL` to the main env var table with default `gpt-4o-mini`

### C5. `OPENAI_BASE_URL` Docker vs Code Default Mismatch

**Files:** `docker-compose.yml`

**Problem:** Docker compose default ends in `/v1`; code default does not.

**Implementation:**
1. Remove `/v1` suffix from the docker-compose default: `https://api.openai.com`
2. `NormalizeBaseURL` in the embedder/generator handles both cases, but consistency is cleaner

---

## Phase 4: Database Persistence Gaps

### D1. `ExpectedAnswer` Not Persisted in `eval_queries`

**Files:** `internal/eval/store.go`, `internal/db/migrations/`

**Problem:** Ground-truth expected answer is used by the judge at eval time but never written to SQL. Results read back via `GetRunResults()` lose this field.

**Implementation:**
1. Add `expected_answer TEXT` column to `eval_queries` (new migration `004_add_expected_answer.sql`)
2. Update `AddQueryResult` INSERT to include `r.ExpectedAnswer`
3. Update `GetRunResults` SELECT and Scan to populate it

**Verification:**
- `go test ./internal/eval/` — existing store tests cover round-trip

### D2. Per-Question `NDCGGraded` Not Persisted

**Files:** `internal/eval/store.go`, `internal/db/migrations/`

**Problem:** Per-question graded NDCG is computed but never stored — only the aggregate survives.

**Implementation:**
1. Add `ndcg_graded DOUBLE PRECISION` column to `eval_queries` (new migration `005_add_ndcg_graded.sql`)
2. Update `AddQueryResult` INSERT to include `r.NDCGGraded`
3. Update `GetRunResults` SELECT and Scan

**Verification:**
- `go test ./internal/eval/`

---

## Phase 5: Cross-CLI Consistency

### E1. `--tag` Env Var Across All CLIs

**Files:** `cmd/eval/main.go`, `cmd/query/main.go`

**Problem:** `preprocess` and `index` respect the `TAG` env var; `eval` and `query` do not.

**Implementation:**
1. Add `TAG` env var support to `cmd/eval/main.go` flag: `config.EnvOrDefault("TAG", "")`
2. Add `TAG` env var support to `cmd/query/main.go` flag: `config.EnvOrDefault("TAG", "")`
3. Update documentation notes that `TAG` is shared across all four CLIs

**Verification:**
- `TAG=my-tag go run ./cmd/eval --help` shows the correct default

### E2. `query` CLI Should Use `config.Load()`

**Files:** `cmd/query/main.go`

**Problem:** `query` does not call `config.Load()`, so it lacks `--log-level`, `--max-retries`, `--retry-backoff` flags. It also duplicates `QDRANT_URL` default logic.

**Implementation:**
1. Call `config.Load()` at the start of `run()` in `cmd/query/main.go`
2. Use `cfg.QdrantURL` and `cfg.QdrantAPIKey` instead of inline `os.Getenv` calls with duplicated defaults
3. This is a minor refactor — `config.Load()` also parses system flags, adding `--log-level` etc. to query

**Verification:**
- `go build ./cmd/query/`
- `go run ./cmd/query --help` shows the new flags

---

## Phase 6: Docker-Compose Cleanup

### F1. Remove Dead RPM Env Vars

**Files:** `docker-compose.yml`

**Problem:** `RPM`, `OPENAI_RPM`, `GEMINI_RPM`, `OPENROUTER_RPM`, `LMSTUDIO_RPM` are set but never consumed.

**Implementation:**
1. Remove all five RPM variables from `docker-compose.yml`
2. Or add a comment noting they are reserved for future rate-limit configuration

### F2. Fix `/v1` Default Mismatch

**Files:** `docker-compose.yml`

**Problem:** `OPENAI_BASE_URL` default includes `/v1` suffix; code default does not.

**Implementation:**
1. Change `https://api.openai.com/v1` → `https://api.openai.com` in docker-compose

---

## Execution Order

```
Phase 1 (CLI Completeness) — unblocks users
    ↓
Phase 2 (Dead Fields) — clean up cruft
    ↓
Phase 4 (DB Persistence) — fix data loss
    ↓
Phase 3 (Documentation) — can start after P1–P2 stabilize
    ↓
Phase 5 (Cross-CLI Consistency) — independent, mechanical
    ↓
Phase 6 (Docker Cleanup) — independent
```

Phases 2, 3, and 6 can partially run in parallel since they touch different files. Phase 4 depends on understanding the schema (migration order).

| Phase | Depends On |
|-------|-----------|
| 1 | — |
| 2 | 1 (minor — avoids conflict on `cmd/eval/main.go`) |
| 3 | — (docs only) |
| 4 | — (new migrations, independent) |
| 5 | 1 (same files) |
| 6 | — |
