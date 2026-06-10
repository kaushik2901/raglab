# Implementation Plan: API-First Architecture

## Goal

Remove all CLI binaries except `api` and `workerd`. Make every pipeline parameter explicit in the API payload — no defaults, no auto-generation, no fallback logic. Only connection strings and credentials remain in env/config.

---

## Phase 1: Remove CLI Binaries

### 1.1 Delete CLI main files

| File                    | Action |
| ----------------------- | ------ |
| `cmd/preprocess/main.go` | Delete |
| `cmd/index/main.go`      | Delete |
| `cmd/query/main.go`      | Delete |
| `cmd/eval/main.go`       | Delete |

### 1.2 Update `make.cmd`

- Remove lines 4-7 (`preprocess`, `index`, `eval`, `query` labels)
- Remove lines 18-20, 24, 26 (build commands for removed CLIs)
- Remove lines 33-75 (`:query`, `:preprocess`, `:eval`, `:index` sections)
- Remove lines 96-107 (`:run` section that references `preprocess.exe`)
- Simplify `:build` label to only build `workerd` + `api`
- Update `:build` success message

### 1.3 Update `AGENTS.md`

- Remove lines 6-9 (build commands for removed CLIs)
- Remove lines 114-125 (CLI usage examples)
- Remove lines 181-182 (eval CLI usage)

### 1.4 Update `README.md`

- Remove CLI usage sections for preprocess/index/eval/query (lines 119-227)
- Keep only `api` and `workerd` build/run instructions

### 1.5 Update `docs/streaming-architecture-design.md`

- Remove CLI references on lines 203-204

---

## Phase 2: Remove Auto-Generation and Default Logic

### 2.1 `internal/config/config.go` — Remove `ResolveTag`

`ResolveTag` auto-generates a tag if none is provided. In the API, the tag must always be explicit.

- Delete the `ResolveTag` function completely

### 2.2 `internal/api/service_workflow.go` — Stop calling `ResolveTag`

- In `InsertPreprocess`: replace `config.ResolveTag(req.Tag, "pre")` with `req.Tag`
- In `InsertIndex`: replace `config.ResolveTag(req.Tag, "idx")` with `req.Tag`
- In `InsertEval`: replace `config.ResolveTag(req.Tag, "eval")` with `req.Tag`

### 2.3 `internal/api/service_chat.go` — Remove env-var-based defaults

`NewChatService` currently reads `LLM_PROVIDER`, `EMBEDDING_PROVIDER`, `LLM_MODEL`, `EMBEDDING_MODEL` from the environment with fallback defaults. This violates the explicit-payload principle.

**Approach:** Make `ChatService` per-request aware by holding a `VectorStore` reference and constructing the embedder/generator/retriever on each `Chat()` call from the `ChatRequest` fields. This means every LLM-related parameter is explicit per request.

Additionally, remove inline defaults:
- `retrieveSources`: remove `if topK <= 0 { topK = 5 }`
- `resolveMaxTokens`: remove `if req.MaxTokens <= 0 { return 1024 }`
- `resolveTemperature`: remove the `Temperature != nil` fallback — field is required

---

## Phase 3: Make All API Payload Fields Required

### 3.1 `internal/api/types.go` — Remove `omitempty`, add required fields

**`PreprocessRequest`:**
- `Tag` — remove `omitempty`
- `IncludeDirs` — remove `omitempty` (can be empty slice if explicit)

**`IndexRequest`:**
- Remove `omitempty` from: `Tag`, `ParserStrategy`, `ChunkStrategy`, `ChunkSize`, `ChunkOverlap`, `EmbeddingProvider`, `EmbeddingModel`, `BatchSize`, `IndexConcurrency`, `DocTimeout`

**`EvalRequest`:**
- Remove `omitempty` from: `Tag`, `TopK`, `Ks`, `LLMProvider`, `LLMModel`, `EmbeddingProvider`, `EmbeddingModel`, `JudgeProvider`, `JudgeModel`, `BatchSize`, `Workers`

**`ChatRequest`:**
- Remove `omitempty` from: `Tag`, `Query`, `ConversationID`, `TopK`, `Temperature`, `MaxTokens`, `LLMProvider`, `LLMModel`, `EmbeddingProvider`, `EmbeddingModel`

### 3.2 Update `Validate()` methods

- **`PreprocessRequest.Validate()`** — add `if r.Tag == "" { return error }`
- **`IndexRequest.Validate()`** — add checks for all fields: `Tag`, `ParserStrategy`, `ChunkStrategy`, `ChunkSize`, `ChunkOverlap`, `EmbeddingProvider`, `EmbeddingModel`, `BatchSize`, `IndexConcurrency`, `DocTimeout`
- **`EvalRequest.Validate()`** — add checks for all fields: `Tag`, `TopK`, `Ks`, `LLMProvider`, `LLMModel`, `EmbeddingProvider`, `EmbeddingModel`, `JudgeProvider`, `JudgeModel`, `BatchSize`, `Workers`
- **`ChatRequest.Validate()`** — add checks for all fields: `Tag`, `Query`, `TopK`, `Temperature`, `MaxTokens`, `LLMProvider`, `LLMModel`, `EmbeddingProvider`, `EmbeddingModel`

---

## Phase 4: Remove Fallback Defaults from Worker Code

### 4.1 `internal/workflow/index_worker.go` — `RunIndexing`

Remove in-code fallback defaults (the caller must provide valid values):

| Current code                                                       | Action      |
| ------------------------------------------------------------------ | ----------- |
| `if concurrency <= 0 { concurrency = 5 }`                          | Remove      |
| `if embeddingProvider == "" { embeddingProvider = ProviderOpenAI }` | Remove      |
| `if parserStrategy == "" { parserStrategy = "markdown" }`          | Remove      |
| `if chunkStrategy == "" { chunkStrategy = "fixed" }`               | Remove      |
| `if docTimeout <= 0 { docTimeout = 30 * time.Minute }`             | Remove      |
| `config.IntEnvOrDefault("MAX_INDEX_FILE_SIZE", ...)`                | Replace with hard-coded constant `100 * 1024 * 1024` |
| `QDRANT_URL` / `QDRANT_API_KEY` env fallback                       | Keep (infrastructure) |

### 4.2 `internal/workflow/eval_worker.go` — `EvalWorker.Work`

| Current code                                        | Action |
| --------------------------------------------------- | ------ |
| `if workers <= 0 { workers = 5 }`                   | Remove |
| `if batchSize <= 0 { batchSize = 20 }`              | Remove |

### 4.3 `internal/workflow/eval_worker.go` — `createEvalDeps`

| Current code                                                       | Action |
| ------------------------------------------------------------------ | ------ |
| `if llmProvider == "" { llmProvider = ProviderOpenAI }`            | Remove |
| `if embeddingProvider == "" { embeddingProvider = ProviderOpenAI }` | Remove |
| `if judgeProvider == "" { judgeProvider = llmProvider }`            | Remove |
| `if embeddingModel == "" { embeddingModel = "text-embedding-3-small" }` | Remove |
| `QDRANT_URL` / `QDRANT_API_KEY` env fallback                       | Keep (infrastructure) |

### 4.4 `internal/workflow/eval_worker.go` — `collectResults`

| Current code                                | Action |
| ------------------------------------------- | ------ |
| `if len(ks) == 0 { ks = []int{1, 3, 5, 10} }` | Remove |

---

## Phase 5: Update Serialization for Workflow Args

The workflow argument structs (`PreprocessArgs`, `IndexArgs`, `EvalArgs`) already have proper `json` tags and serialize correctly through River. `IndexArgs.DocTimeout` is `time.Duration` which River handles as nanoseconds.

**No changes needed** — this is a verification step only.

---

## Phase 6: Update Tests

### 6.1 `internal/api/service_workflow_test.go`

- Update test payloads to include all required fields
- Ensure `TestInsertIndex_Success`, `TestInsertEval_Success` pass full payloads

### 6.2 `internal/api/handler_workflow_test.go`

- Update test JSON bodies to include all required fields
- Add/migrate tests for missing-field validation

### 6.3 `internal/api/handler_chat_stream_test.go`

- Update test JSON bodies to include all required chat fields (temperature, max_tokens, etc.)

### 6.4 Other tests

- No changes needed for tests that construct `ChatService` with mocks directly (they bypass `NewChatService`)
- No changes needed for `handler_workflow_test.go` missing-field tests — they will still pass since validation catches empty fields

---

## Phase 7: Remove Unused Config Helpers

### Remove from `internal/config/config.go`:

| Function           | Reason                                             |
| ------------------ | -------------------------------------------------- |
| `ResolveTag`       | Phase 2.1 — tag auto-generation removed            |
| `FloatEnvOrDefault` | Only used by removed CLIs; now unused              |

### Keep (still in use):

| Function              | Used by                                |
| --------------------- | -------------------------------------- |
| `EnvOrDefault`         | `config.Load()` — connection strings   |
| `IntEnvOrDefault`      | `workerd/main.go` — `WORKER_CONCURRENCY` |
| `DurationEnvOrDefault`  | `config.Load()` — `RETRY_BACKOFF`, `API_REQUEST_TIMEOUT` |

---

## Phase 8: Verify Build and Tests

```powershell
go build ./...       # no compilation errors
go vet ./...         # no issues
go test ./...        # all tests pass
```

---

## Summary of Files Changed

| File                                             | Change |
| ------------------------------------------------ | ------ |
| `cmd/preprocess/main.go`                          | DELETE |
| `cmd/index/main.go`                               | DELETE |
| `cmd/query/main.go`                               | DELETE |
| `cmd/eval/main.go`                                | DELETE |
| `make.cmd`                                        | Remove CLI builds/targets |
| `AGENTS.md`                                       | Remove CLI references |
| `README.md`                                       | Remove CLI usage sections |
| `docs/streaming-architecture-design.md`            | Remove CLI refs in examples |
| `internal/config/config.go`                       | Remove `ResolveTag`, `FloatEnvOrDefault` |
| `internal/api/types.go`                           | Remove `omitempty`, add required fields |
| `internal/api/service_workflow.go`                | Remove `config.ResolveTag` calls |
| `internal/api/service_chat.go`                    | Remove env defaults, make params explicit |
| `internal/workflow/index_worker.go`               | Remove fallback defaults |
| `internal/workflow/eval_worker.go`                | Remove fallback defaults |
| `internal/api/service_workflow_test.go`            | Update for required fields |
| `internal/api/handler_workflow_test.go`            | Update for required fields |
| `internal/api/handler_chat_stream_test.go`         | Update for required fields |
