# HTTP API Layer — Phase-Based Implementation Plan

*Derived from `docs/api-layer-analysis.md`. Phases are ordered by dependency — later phases assume earlier ones are complete.*

---

## Phase 0 — Quick Wins (No Dependencies)

### 0.1 Remove CORS Middleware
**Files:** `internal/api/middleware.go:76-87`, `internal/api/server.go:50`

- Delete the `CORS` function from `middleware.go`
- Remove `r.Use(CORS)` from `server.go`
- Also remove `PUT`/`DELETE` from the function (it's already unused, but don't leave orphaned lists)
- **Remove tests** in `middleware_test.go:88-113` (`TestCORS_Simple`, `TestCORS_Options`)

### 0.2 Clean Up `WorkflowService` Constructors
**File:** `internal/api/service_workflow.go:36-42`

- Delete `NewWorkflowService(client *river.Client[pgx.Tx])` (unused)
- Rename `NewWorkflowServiceWithClient` → `NewWorkflowService`
- Update the caller in `server.go:98` from `NewWorkflowServiceWithClient` → `NewWorkflowService`

### 0.3 Add `ChatRequest.Validate()`
**Files:** `internal/api/types.go:89-100`, `internal/api/handler_chat.go:14-21`

- Add `func (r ChatRequest) Validate() error` to `types.go` — check `Query` and `Tag` are non-empty
- Replace inline validation in `handler_chat.go:14-21` with `req.Validate()`
- Also apply in `handler_chat_stream.go:35-38` — replace the inline `if req.Query == "" || req.Tag == ""` with `req.Validate()`

### 0.4 `db.Connect()` Accepts DSN Parameter
**File:** `internal/db/db.go:14-18`, `internal/api/server.go:34`

- Change signature: `func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error)`
- Remove the inline `os.Getenv("DATABASE_URL")` + default from `db.go`
- Update caller in `server.go:34`: `db.Connect(context.Background(), cfg.DatabaseURL)`

### 0.5 `parseDocTimeout` Log on Bad Input
**File:** `internal/api/service_workflow.go:16-25`

- Add `slog.Warn` when `time.ParseDuration` fails, including the raw value
- Alternatively, return an error and let the caller decide — but logging is safer for backward compatibility

---

## Phase 1 — Config & Connection Wiring

### 1.1 Inject `*config.Config` and `qstore.VectorStore` into `ChatService`
**Files:** `internal/api/service_chat.go:32-64`, `internal/api/server.go:78-85`, `internal/api/types.go`

This unifies the Qdrant connection and stops `ChatService` from self-initializing.

- Change `NewChatService()` → `NewChatService(cfg *config.Config, vs qstore.VectorStore) (*ChatService, error)`
- Remove the inline `config.EnvOrDefault()` calls and `qs := qstore.NewQdrantStore(...)` / `qs.Connect(...)` from `NewChatService`
- Use `cfg` + `config.ResolveProviderConfig()` for LLM/embedder provider resolution
- Update caller in `server.go:78`: `chatSvc, err := NewChatService(cfg, s.qdrant)`
- **Result:** One Qdrant connection, shared config path, single source of truth

### 1.2 Make Timeout Configurable
**File:** `internal/api/server.go:49`, `internal/config/config.go`

- Add `APIRequestTimeout` field to `config.Config` (read from `API_REQUEST_TIMEOUT` env var, default `60s`)
- Replace hardcoded `60 * time.Second` with `cfg.APIRequestTimeout`
- The stream handler may need a longer value — document this

### 1.3 Make Chat Memory Capacity Configurable
**File:** `internal/api/service_chat.go:62`, `internal/config/config.go`

- Add `ChatMemorySize` to `config.Config` (`CHAT_MEMORY_SIZE`, default `10`)
- Pass into `ChatService` and use instead of hardcoded `10`
- The ring buffer allocates at startup; memory impact is negligible

### 1.4 Make Artifact Path Configurable
**File:** `internal/api/handler_artifact.go:14`, `internal/config/config.go`

- Add `ArtifactsDir` to `config.Config` (`ARTIFACTS_DIR`, default `artifacts`)
- Pass into `Server` and store as a field, or read from config in the handler
- `Server` already has `cfg`, so access via `s.cfg.ArtifactsDir`

---

## Phase 2 — Error Handling Standardization

### 2.1 Replace Envelope with RFC 9457 Problem Details
**Files:** `internal/api/response.go`, all 9 handler files (32 call sites)

**Structural change** — the entire error response format:

- **Before:** `envelope{Data, Error}` wrapper; `respondJSON` always wraps in `{"data":..., "error":...}`
- **After:** `ProblemDetail` struct is the direct response body; `respondJSON` becomes unnecessary for errors; success responses still use `{"data":...}` but via a simpler helper

**Implementation steps:**

1. Define `ProblemDetail` in `response.go`:
   ```go
   type ProblemDetail struct {
       Type     string `json:"type"`
       Title    string `json:"title"`
       Status   int    `json:"status"`
       Detail   string `json:"detail"`
       Instance string `json:"instance,omitempty"`
   }
   ```

2. New helper: `respondProblem(w http.ResponseWriter, status int, title, detail string)` — writes `ProblemDetail` as `application/problem+json`. Derive `Type` from the error code convention (e.g., `/errors/invalid-json`).

3. Remove the `envelope` struct. Replace `respondJSON` with a variant that writes `{"data":...}` directly (no `envelope` wrapper).

4. Update all handlers (32 call sites):
   - `respondError(w, 400, "INVALID_JSON", "...")` → `respondProblem(w, 400, "Invalid Request Body", "...")`
   - `respondError(w, 500, "INTERNAL_ERROR", "...")` → `respondProblem(w, 500, "Internal Server Error", "...")`

5. Update `respondJSON` call sites to write `{"data": data}` instead of `{"data": data, "error": null}`.

6. Update all tests that assert on the old envelope structure (especially `response_test.go`, `handler_workflow_test.go`, `handler_health_test.go`, `middleware_test.go`).

### 2.2 Handle SSE Error Status Codes
**File:** `internal/api/handler_chat_stream.go`

- For client errors (bad JSON, missing fields): write HTTP 400 before switching to SSE mode
- For server errors (retrieval/generation failure): keep current behavior (200 with error event) since headers are already flushed
- Alternatively, use a two-phase write: validate first, then write SSE headers only after validation passes

Option: move the validation before the SSE header setup, and use `respondProblem` for bad requests:

```go
if err := req.Validate(); err != nil {
    respondProblem(w, 400, "Bad Request", err.Error())
    return
}
// Now set SSE headers and proceed
```

### 2.3 Timeout Middleware Writes Error Response
**File:** `internal/api/middleware.go:66-73`

Replace the custom `Timeout` with chi's built-in `chi.Timeout(duration)`, which writes a 503 Service Unavailable response when the context times out. Or enhance the custom version to detect context cancellation and write a problem response.

Option A (simplest): Replace `r.Use(Timeout(60*time.Second))` with `r.Use(chi.Timeout(duration))` and delete the custom `Timeout` function.

### 2.4 No Request Body Size Limit
**Files:** All handler files that call `json.NewDecoder(r.Body)`

Add `http.MaxBytesReader` protection to every handler. Pattern:

```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    respondProblem(w, 400, "Bad Request", "invalid request body")
    return
}
```

A middleware-based approach would be cleaner:
- Add a `MaxBodySize(size int64) func(http.Handler) http.Handler` middleware
- Apply it to all routes (except `/health` and `/chat/stream` which need streaming)
- Pass the limit from config (`MAX_BODY_SIZE`, default `1MB`)

### 2.5 `respondJSON` Logs Write Errors
**File:** `internal/api/response.go`

- Capture the error from `json.NewEncoder(w).Encode(...)` and log with `slog.Warn` when non-nil
- Don't change behavior — just add visibility

---

## Phase 3 — DRY & Refactoring

### 3.1 Extract Shared RAG Logic
**Files:** `internal/api/service_chat.go`, `internal/api/handler_chat_stream.go`

**Goal:** Eliminate the ~67 lines of duplicated RAG orchestration.

Extract these methods on `ChatService`:

```go
func (s *ChatService) buildMessages(req ChatRequest, results []types.SearchResult) []openai.ChatCompletionMessageParamUnion {
    // system message + memory turns + context + question
}

func (s *ChatService) retrieveSources(ctx context.Context, req ChatRequest) ([]types.SearchResult, []SourceDoc, error) {
    // retrieve + convert to SourceDoc
}
```

**Revised `ChatService.Chat()`** becomes:
```go
func (s *ChatService) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
    start := time.Now()
    results, sources, err := s.retrieveSources(ctx, req)
    if err != nil { return nil, err }
    messages := s.buildMessages(req, results)
    // call generator, build response, manage memory
}
```

**Revised `chatStreamHandler`** becomes:
```go
results, sources, err := s.chat.retrieveSources(ctx, req)
if err != nil { sendEvent("error", ...); return }
sendEvent("retrieval", map[string]any{"results": sources})

messages := s.chat.buildMessages(req, results)
// generate stream, emit tokens, emit done event
```

### 3.2 Split Server God Object
**Files:** `internal/api/server.go`, new `router_*.go` files

Not a functional change — purely organizational. Each route group becomes a separate file with a registration function:

```
internal/api/
  handler_chat.go        → chat routes in server.go
  handler_chat_stream.go  ↓ (unchanged)
  handler_eval.go         ↓
  handler_workflow.go     ↓
  handler_health.go       ↓
  handler_artifact.go     ↓
```

Proposed split pattern — create a `Router` interface per group:

```go
// router_workflow.go
type WorkflowRouter struct {
    svc *WorkflowService
}

func NewWorkflowRouter(svc *WorkflowService) *WorkflowRouter { ... }

func (r *WorkflowRouter) Register(mux chi.Router) {
    mux.Post("/preprocess", r.preprocessHandler)
    mux.Post("/index", r.indexHandler)
    mux.Post("/eval", r.evalHandler)
    mux.Get("/{id}", r.workflowStatusHandler)
}
```

Then in `server.go`:
```go
NewWorkflowRouter(s.workflows).Register(r.Route("/api/v1/workflows"))
```

**Files to create:**
- `router_workflow.go` — moves `preprocessHandler`, `indexHandler`, `evalHandler`, `workflowStatusHandler`, `WorkflowRouter`
- `router_eval.go` — moves `evalListHandler`, `evalDetailHandler`, `evalCompareHandler`, `EvalRouter`
- `router_chat.go` — moves `chatHandler`, `chatStreamHandler`, `ChatRouter`  (keeps server's `chat` field init)
- `router_health.go` — moves `healthHandler`, `HealthRouter`
- `router_artifact.go` — moves `artifactListHandler`, `ArtifactRouter`

**Result:** `Server` struct shrinks to just `cfg`, `router`, `http`, `pool`, `qdrant`. Each sub-router owns its handler methods and dependencies.

---

## Phase 4 — Eval Optimizations

### 4.1 Add `GetRuns(ctx, ids []string)` for Batch Lookup
**File:** `internal/api/service_eval.go`, `internal/api/handler_eval.go`

- Add method: `func (s *EvalService) GetRuns(ctx context.Context, ids []string) (map[string]RunSummary, error)`
- Use `WHERE id = ANY($1)` with `pgx`'s `pgtype.FlatArray` or a simple SQL `IN` clause
- Update `evalCompareHandler` to call `GetRuns` once instead of looping `GetRun`

### 4.2 Add `GetRunSummary(ctx, id)` for Lightweight Lookup
**File:** `internal/api/service_eval.go`

- Add method returning only `RunSummary` (no questions query)
- `evalCompareHandler` uses this (via the batch `GetRuns`) instead of `GetRun`

### 4.3 Limit `compare_to` Parameters
**File:** `internal/api/handler_eval.go`

- Cap the number of accepted `?compare_to=` parameters (e.g., max 5)
- Return `respondProblem(w, 400, ...)` if exceeded

---

## Phase 5 — Polish & Testing

### 5.1 Add Request Context to Recovery Logging
**File:** `internal/api/middleware.go:54-64`

```go
slog.Error("panic",
    "err", rec,
    "stack", string(debug.Stack()),
    "method", r.Method,
    "path", r.URL.Path,
    "request_id", r.Context().Value(ctxKeyRequestID),
)
```

### 5.2 Reduce Test Boilerplate
**File:** `internal/api/handler_workflow_test.go` (and others)

Add a `testServer` helper:

```go
type testServerOpts struct {
    workflows *WorkflowService
    evalSvc   *EvalService
    chat      *ChatService
    pool      *pgxpool.Pool
    qdrant    qstore.VectorStore
}

func newTestServer(opts testServerOpts) *Server {
    return &Server{
        workflows: opts.workflows,
        evalSvc:   opts.evalSvc,
        chat:      opts.chat,
        pool:      opts.pool,
        qdrant:    opts.qdrant,
    }
}
```

Then tests become:
```go
s := newTestServer(testServerOpts{
    workflows: &WorkflowService{client: &mockJobClient{...}},
})
```

### 5.3 Update All Tests for Phase 2 Changes
- `response_test.go` — rewrite to match `ProblemDetail` format
- `handler_workflow_test.go` — update envelope assertions
- `handler_health_test.go` — update envelope assertions
- `middleware_test.go` — update recovery test assertion for new error format + remove CORS tests
- `handler_artifact_test.go` — update if envelope format changed
- `handler_chat_stream_test.go` — update SSE error assertions

### 5.4 Verify Build & Tests
```powershell
go build -o bin\api.exe .\cmd\api
go test ./internal/api/... -v
go vet ./...
```

---

## Phase Dependency Graph

```
Phase 0 (no deps)
  ├── 0.1 Remove CORS
  ├── 0.2 WorkflowService ctors
  ├── 0.3 ChatRequest.Validate()
  ├── 0.4 db.Connect DSN param
  └── 0.5 parseDocTimeout log

Phase 1 (needs 0.4)
  ├── 1.1 Inject config + Qdrant → ChatService
  ├── 1.2 Make timeout configurable
  ├── 1.3 Make memory size configurable
  └── 1.4 Make artifact path configurable

Phase 2 (needs 0.3)
  ├── 2.1 RFC 9457 Problem Details
  ├── 2.2 SSE error codes
  ├── 2.3 Timeout writes error
  ├── 2.4 MaxBodySize middleware
  └── 2.5 Log write errors

Phase 3 (needs 1.1, 0.3)
  ├── 3.1 Shared RAG logic (ChatService ↔ Stream)
  └── 3.2 Split Server god object

Phase 4 (standalone)
  ├── 4.1 GetRuns batch query
  ├── 4.2 GetRunSummary lightweight
  └── 4.3 Limit compare_to params

Phase 5 (needs 2.1, 0.1, 3.2)
  ├── 5.1 Recovery logging
  ├── 5.2 Test boilerplate helper
  ├── 5.3 Update all tests
  └── 5.4 Build & verify
```

---

## Files Touched Per Phase

| Phase | Files |
|-------|-------|
| 0 | `middleware.go`, `middleware_test.go`, `service_workflow.go`, `server.go`, `types.go`, `handler_chat.go`, `handler_chat_stream.go`, `db.go`, `service_workflow.go` |
| 1 | `service_chat.go`, `server.go`, `config.go`, `handler_artifact.go` |
| 2 | `response.go`, `response_test.go`, all handler files, `middleware.go`, `middleware_test.go`, all handler test files |
| 3 | `service_chat.go`, `handler_chat_stream.go`, new `router_*.go` files, `server.go` |
| 4 | `service_eval.go`, `handler_eval.go` |
| 5 | `middleware.go`, `middleware_test.go`, all test files |

---

## Approximate Effort

| Phase | Est. time | Risk |
|-------|-----------|------|
| Phase 0 | 30 min | Low |
| Phase 1 | 1 hr | Low |
| Phase 2 | 2–3 hr | Medium (touches every handler, every test) |
| Phase 3 | 2 hr | Medium (DRY can introduce regressions if not careful) |
| Phase 4 | 45 min | Low |
| Phase 5 | 1 hr | Low |
| **Total** | **~8 hr** | |
