# Implementation Plan: Codebase Analysis Remediation

> Based on: `docs/codebase-analysis.md`
> Created: 2026-06-07

## Phase Overview

| Phase | Focus | Items | Est. Effort | Priority |
|-------|-------|-------|-------------|----------|
| 1 | Critical Bug Fixes | 1.1, 1.2 | 3-5 days | 🔴 Critical |
| 2 | Race Conditions & Concurrency | 2.1, 2.2 | 1-2 days | 🔴 High |
| 3 | Error Handling & Robustness | 3.1–3.5 | 3-4 days | 🟡 Medium |
| 4 | Code Quality & Deduplication | 4.1–4.8 | 3-5 days | 🟡 Medium |
| 5 | Configuration & Security | 6.1–6.3, 7.1–7.3 | 2-3 days | 🟡 Medium |
| 6 | Testing & Verification | 5.1–5.3 | 4-6 days | 🟢 Low |
| 7 | Documentation & Polish | 8.1–8.2 | 1 day | 🟢 Low |
| 8 | Architectural Improvements | 9.1, 9.2, 9.4 | 5-8 days | 🟢 Low |

**Total estimated effort:** 22-34 days

---

## Phase 1: Critical Bug Fixes

### 1.1 Embedder: HTTP Request Body Consumed on Retry

**Files:** `internal/embedder/openai.go`

**Problem:** `http.Request.Body` is consumed on first `client.Do()` call, subsequent 429 retries send empty payload.

**Implementation:**
1. Move `http.NewRequestWithContext` inside the retry loop so a fresh `bytes.NewReader(body)` is created per attempt
2. Alternatively, wrap the body with `io.NopCloser(bytes.NewReader(body))` at each retry
3. Add a test that verifies request body on retry (covers 5.3)

**Verification:**
- `go test ./internal/embedder/ -v`
- `-race` flag to confirm no new races

### 1.2 Chunk ID Hash Collision — FNV-1a 64-bit

**Files:** `internal/store/qdrant.go`

**Problem:** FNV-1a 64-bit hash for chunk IDs guarantees collisions at corpus scale → silent data loss.

**Implementation:**
1. Replace `chunkIDToUint64` with UUID-based `PointId` (`&qdrant.PointId_Uuid{...}`)
2. Generate a deterministic UUID v5 from the chunk ID string (namespace = DNS `"rag.gitlab-handbook"`)
3. Update all callers of `chunkIDToUint64` to use the new approach
4. Verify no other code depends on the `uint64` return type

**Verification:**
- `go test ./internal/store/ -v`
- Manual Qdrant upsert test with known-colliding inputs

### Dependencies
- Phase 2 must wait for Phase 1 (avoid introducing new races while fixing data loss)
- Phase 6.3 (embedder retry test) depends on 1.1

---

## Phase 2: Race Conditions & Concurrency

### 2.1 Embedder: Data Race on `e.dimensions`

**Files:** `internal/embedder/openai.go`

**Problem:** `e.dimensions` written without synchronization inside `embedBatch`, called concurrently via `errgroup`.

**Implementation:**
1. Replace `int` field with `atomic.Int32`
2. Use `e.dimensions.CompareAndSwap(0, len(d.Embedding))` to ensure only first write succeeds
3. Update `Dimensions()` method to use `e.dimensions.Load()`

### 2.2 `gitUpdate` Does Not `checkout main` Before Pull

**Files:** `internal/stage/clone.go`

**Problem:** `git fetch --all` + `git pull --ff-only` without branch checkout → may pull wrong branch.

**Implementation:**
1. Add `git checkout main` (or configurable default branch) before `git pull`
2. Handle error from `checkout` — if branch doesn't exist, fall back to the remote tracking branch

### Verification (both)
- `go test -race ./internal/embedder/ ./internal/stage/ -v`

---

## Phase 3: Error Handling & Robustness

### 3.1 Index Worker: Parse/Chunk Failures Silently Skipped

**Files:** `internal/workflow/index_worker.go`

**Problem:** `parser.ParseFile` and chunker errors are logged at `Warn` but return `nil` — pipeline reports success despite incomplete index.

**Implementation:**
1. Collect per-file errors in a `[]error` slice instead of returning `nil`
2. Return the aggregate error (or log a summary) after processing all files
3. Include count of skipped files in the final log/output

### 3.2 Verify Stage: Walk Errors Ignored in All 6 Checks

**Files:** `internal/stage/verify.go`

**Problem:** `filepath.Walk` callbacks return `nil` on error, masking walk failures.

**Implementation:**
1. Change all 6 check functions to propagate walk errors
2. Check the return value of `filepath.Walk` and include it in the check result
3. Ensure a single bad directory doesn't terminate the walk early (use `filepath.WalkDir` + proper error handling)

### 3.3 Git Clone: Stdout/Stderr Discarded

**Files:** `internal/stage/clone.go`

**Problem:** Git output goes to process stdout/stderr, not captured in worker context.

**Implementation:**
1. Replace `cmd.Stdout = os.Stdout` / `cmd.Stderr = os.Stderr` with `bytes.Buffer` capture
2. On failure, include captured output in the returned error
3. On success, log at `Debug` level

### 3.4 No Input Validation in Eval CLI

**Files:** `cmd/eval/main.go`

**Problem:** `--dataset-dir` walks blindly for `.json`; failures surface deep in River worker after queuing.

**Implementation:**
1. After walking the dataset directory, attempt to decode each `.json` file
2. Validate required fields (e.g., `questions`, `document_path`)
3. Fail fast before submitting River jobs

### 3.5 Generator: `openai-go` Client's Internal Retry + External Retry Loop

**Files:** `internal/generator/generator.go`

**Problem:** Double retry — `openai-go` SDK retries internally, then external loop retries again.

**Implementation:**
1. Add `option.WithMaxRetries(0)` to the `openai-go` client configuration
2. Keep the external retry loop as the single source of retry logic
3. Verify no other `openai-go` options trigger internal retries

### Verification
- `go test ./internal/workflow/ ./internal/stage/ ./internal/eval/ ./internal/generator/ -v`

### Dependencies
- None

---

## Phase 4: Code Quality & Deduplication

### 4.1 Duplicate `normalizeBaseURL` Function

**Files:** `internal/embedder/openai.go`, `internal/generator/generator.go`

**Implementation:**
1. Extract to `internal/config/normalize.go` (or a new shared `internal/util/` package)
2. Update both callers
3. Add a unit test for edge cases: missing scheme, trailing slash, `/v1` suffix, empty string

### 4.2 Duplicate `parseRetryAfter` Function

**Files:** `internal/embedder/openai.go`, `internal/generator/generator.go`

**Implementation:**
1. Extract alongside `normalizeBaseURL` in shared package
2. Update both callers
3. Add unit tests for: `Retry-After: 120`, `Retry-After: Fri, 31 Dec 1999 23:59:59 GMT`, missing header, invalid value

### 4.3 Duplicate `envOrDefault` / `intEnvOrDefault` / `durationEnvOrDefault`

**Files:** `internal/config/config.go`

**Implementation:**
1. Remove the private duplicates (`envOrDefault`, `intEnvOrDefault`, `durationEnvOrDefault`)
2. Update the two internal callers to use the public versions
3. Keep public versions as the single source

### 4.4 Duplicate `min` Function — Replace with Builtin

**Files:** `internal/eval/metrics.go`

**Implementation:**
1. Remove the local `min` function
2. Replace all calls with the Go 1.21+ builtin `min`

### 4.5 Verify Stage: Extract `walkMarkdownFiles` Helper

**Files:** `internal/stage/verify.go`

**Implementation:**
1. Extract the shared `filepath.Walk` skeleton into `walkMarkdownFiles(dir string, fn func(path string) error) error`
2. Update all 6 check functions + `computeTotalSize` to use the helper
3. Handle walk errors in one place

### 4.6 Package Name Mismatch: `stageimport` → `stage`

**Files:** `internal/stage/*.go` (all files)

**Implementation:**
1. Change `package stageimport` to `package stage` in all files
2. Update all imports across the codebase (`internal/workflow/`, `internal/stage/verify_test.go`, `cmd/preprocess/main.go`, etc.)
3. Run `go build ./...` to catch any missed imports

### 4.7 Legacy Evaluator Dependency

**Files:** `internal/eval/pipeline.go`, `internal/eval/retrieval.go`

**Implementation:**
1. Move `computeNDCGGradedForQuestion` from `retrieval.go` into `pipeline.go` (or a new `metrics.go` in `internal/eval/`)
2. Verify no other code references `retrieval.go` functions
3. Mark `retrieval.go` as truly removable once dependency is broken

### 4.8 Hardcoded Constants

| Location | Constant | Fix |
|----------|----------|-----|
| `eval_worker.go:107` | Batch size `20` | Make configurable via `EvalPipelineArgs` |
| `eval_worker.go:105` | Model `"text-embedding-3-small"` | Remove hardcoded fallback; use `--embedding-model` from CLI |
| `eval/pipeline.go:169-170` | System prompt | Extract to shared constant, deduplicate with `retrieval.go:141-142` |
| `verify.go:233` | `minContent := 50` | Make configurable or define as named constant |

### Verification
- `go build ./...`
- `go test ./... -v`

### Dependencies
- None (independent refactoring)

---

## Phase 5: Configuration & Security Hardening

### 6.1 `LLM_BASE_URL` Default Without `/v1` Suffix

**Files:** `internal/config/config.go`

**Problem:** Default `"https://api.openai.com/v1"` is confusing since `normalizeBaseURL` strips it.

**Implementation:**
1. Change default to `"https://api.openai.com"` (no `/v1`)
2. Verify `normalizeBaseURL` still produces correct URLs
3. Update any tests that depend on the old default

### 6.2 Embedding Batch Size Consistently Configurable

**Files:** `internal/workflow/eval_worker.go`

**Problem:** Eval worker hardcodes batch size `20`; `PipelineArgs.EmbedBatch` field exists but unused.

**Implementation:**
1. Wire `EvalPipelineArgs.EmbedBatch` through the eval worker
2. Pass it to `embedder.New()` in the eval pipeline
3. Remove hardcoded `20` on `eval_worker.go:107`

### 6.3 Deadline/Timeout Propagation for Workers

**Files:** `internal/workflow/index_worker.go`, `internal/eval/pipeline.go`

**Problem:** No deadline derived from River job context — network hangs block indefinitely.

**Implementation:**
1. In `RunIndexing` and `Evaluate`, derive a timeout from the River job context
2. Use `context.WithTimeout` per-document or per-batch with sensible defaults
3. Log deadline exceeded as distinct from other errors

### 7.1 `.env` Not in `.gitignore`

**Files:** `.gitignore`

**Implementation:**
1. Add `.env` to `.gitignore`

### 7.2 API Key Leakage in Error Paths

**Files:** Various

**Problem:** Keys can leak via panic traces and `fmt.Errorf` chains.

**Implementation:**
1. Audit `fmt.Errorf` calls for API key inclusion
2. Ensure config logging redacts sensitive fields (add `fmt.Stringer` to config that hides key values)
3. Add a note about process memory exposure in security docs

### 7.3 No TLS Gating for Non-OpenAI Providers

**Problem:** Cleartext transmission for `http://` URLs and `lmstudio` provider.

**Implementation:**
1. Add a warning log when API keys are used over non-TLS connections
2. Optionally: require `--allow-insecure` flag to skip the warning

### Verification
- `go build ./...`
- `go test ./... -v`

---

## Phase 6: Testing & Verification

### 5.1 Missing Test: `internal/eval/store_test.go`

**Implementation:**
1. Reuse `connectOrSkip` pattern from `internal/workflow/store_test.go`
2. Test all 5 CRUD methods: `CreateRun`, `AddQueryResult`, `UpdateRunMetrics`, `GetRunResults`
3. Test edge cases: empty results, missing run, concurrent writes

### 5.2 Worker Business Logic Tests

**Files:** `internal/workflow/preprocess_worker_test.go`, `internal/workflow/index_worker_test.go`, `internal/workflow/eval_worker_test.go` (new)

**Implementation:**
1. **Preprocess worker:** Test stage execution, state transitions, error propagation
2. **Index worker:** Test document processing loop, error collection, Qdrant upsert flow
3. **Eval worker:** Test pipeline orchestration, result collection, metrics aggregation
4. Use River's `rivertest` package (if available) or mock the client

### 5.3 Embedder Retry Body Test

**Files:** `internal/embedder/openai_test.go`

**Implementation:**
1. Add a mock HTTP handler that returns 429 for first N attempts
2. Verify that each retry sends the identical request body
3. Verify failure after exhausting retries

### Dependencies
- Phase 6.3 depends on Phase 1.1 (body reuse fix must be in place first)

---

## Phase 7: Documentation & Polish

### 8.1 AGENTS.md: Outdated Statements

**Files:** `AGENTS.md`

**Updates needed:**
1. "Zero external Go dependencies" → update to reflect current `go.sum` state
2. "`build.cmd` does NOT exist" → verify current state of `build.cmd` and README
3. Ensure any other stale statements are corrected

### 8.2 Missing: Embedding Model Dimension Mismatch

**Files:** `docs/` or `AGENTS.md`

**Implementation:**
1. Document the behavior when `EnsureCollection` encounters a dimension mismatch
2. Add a note about the need to re-create the Qdrant collection when changing embedding models
3. Optionally: add a `--force-recreate` flag to the index CLI

### Verification
- Review rendered markdown

---

## Phase 8: Architectural Improvements (Optional/Future)

### 9.1 Polling-Based Workflow Completion

**Problem:** CLI polls every 2s — inefficient for long runs.

**Idea:** Use River's LISTEN/NOTIFY or a callback/webhook mechanism.

**Status:** Deferred — not blocking correctness.

---

### 9.2 Embedder + Generator Unification

**Problem:** Embedder (raw `net/http`) and generator (`openai-go`) implement same retry logic on different stacks.

**Idea:** Either:
- Migrate embedder to `openai-go` SDK (gains consistency, loses manual retry code)
- Or extract a shared HTTP client with retry middleware

**Status:** Deferred — Phase 4 deduplication (4.1, 4.2) is the tactical fix.

---

### 9.4 Structured Logging with Trace/Request IDs

**Problem:** Interleaved log lines from concurrent River jobs cannot be correlated.

**Idea:** Add `slog.With("workflow_id", id)` at River worker entry points.

**Status:** Deferred — not blocking correctness, but high value for debugging.

---

## Execution Order

```
Phase 1 (Critical Bugs)
    ↓
Phase 2 (Race Conditions)
    ↓
Phase 3 (Error Handling)        Phase 4 (Code Quality) — parallelizable
    ↓                                   ↓
Phase 5 (Config & Security) — Phase 5 can start after Phase 3
    ↓
Phase 6 (Testing) — depends on Phase 1 (6.3), Phase 3 (6.1-6.2)
    ↓
Phase 7 (Documentation) — can start after all code changes stabilize
    ↓
Phase 8 (Architectural) — independent, can be deferred
```

Phases 3 and 4 can run in parallel since they touch different files. Phase 5 has some overlap with Phase 3 (error handling) but is largely independent. Phase 6 is best done after Phases 1-5 to test the fixes.
