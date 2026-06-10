# HTTP API Layer — Improvement Analysis

_Generated from code review of `internal/api/` (20 files), `cmd/api/`, `internal/config/`, `internal/db/`, and `internal/store/`_

---

## 1. Duplicate Qdrant Connection (ChatService Bypasses Server)

`ChatService.NewChatService()` at `internal/api/service_chat.go:45-48` creates its own Qdrant gRPC connection by re-reading env vars directly, while the `Server` at `server.go:39` already opens one. This means two separate gRPC connections to Qdrant from the same process.

**Fix:** Pass the `Server`'s `qstore.VectorStore` (and the `*config.Config`) into `ChatService` instead of letting it self-initialize. This also eliminates the duplicated env-var resolution.

---

## 2. RAG Logic Duplicated Between Chat and Stream

The core RAG flow (retrieval, source-doc conversion, memory-injection, prompt building, answer post-processing) is copy-pasted between:

| File                     | Line range                      |
| ------------------------ | ------------------------------- |
| `service_chat.go`        | `service_chat.go:69-139`        |
| `handler_chat_stream.go` | `handler_chat_stream.go:42-108` |

These are almost identical except the stream handler calls `GenerateStream` and emits SSE events. The common parts should be extracted into a shared method on `ChatService` (e.g., `buildMessages`, `buildResponse`) that both invocation paths call.

**Concretely:**

- Extract `buildConversationMessages(req, results) []openai.ChatCompletionMessageParamUnion`
- Extract `convertResultsToSources(results) []SourceDoc`
- Extract the `chat(ctx, req) -> (answer, sources, usage, error)` core from `ChatService.Chat()`
- The stream handler calls the shared retrieval + message building, then calls `GenerateStream` and emits events from the callback

---

## 3. Error Handling Inconsistencies

### 3a. Request validation pattern

`ChatRequest` has no `Validate()` method — validation is inline at `handler_chat.go:14-21`. All other request types (`PreprocessRequest`, `IndexRequest`, `EvalRequest`) use `Validate()`. The pattern should be consistent.

### 3b. SSE returns 200 on error

`chatStreamHandler` writes headers for SSE (200 status), then sends `error` events. Tests assert `rec.Code == 200` even on error. While this is technically valid SSE (the event stream itself succeeded), HTTP-level error codes would be more honest for certain error types (e.g., 400 for bad JSON). The current behavior means downstream clients must parse the event stream to detect errors.

### 3c. Use RFC 9457 Problem Details for errors

The current custom envelope format (`{"data": ..., "error": {"code": ..., "message": ...}}`) should be replaced with the standard `application/problem+json` format (RFC 9457):

```json
{
  "type": "/errors/invalid-json",
  "title": "Invalid Request Body",
  "status": 400,
  "detail": "invalid request body",
  "instance": "/api/v1/workflows/preprocess"
}
```

Benefits:
- Industry standard (used by all major API gateways)
- `status` field eliminates manual status-code tracking
- `type` URI is extensible with docs links
- Nginx/kong/envoy can parse and rewrite problem responses natively
- No breaking change for the JSON structure (still one envelope)

Implementation touches every `respondError` call (32 usages across 9 files) plus `response.go`. The `APIError` struct becomes `ProblemDetail`, and the `envelope` wrapper disappears entirely since the problem IS the response body.

---

## 4. Eval Compare Handler Has N+1 Query Problem

`handler_eval.go:43-56` loops over each comparison ID and calls `evalSvc.GetRun()` individually, each of which runs two SQL queries (one for the run, one for the question count). This is an N+1 pattern.

**Fix:** Add a `GetRuns(ctx, ids []string) ([]RunSummary, error)` method to `EvalService` that queries `WHERE id = ANY($1)` in a single round-trip.

---

## 5. `WorkflowService` Constructor Proliferation

`service_workflow.go` defines two constructors:

```go
func NewWorkflowService(client *river.Client[pgx.Tx]) *WorkflowService
func NewWorkflowServiceWithClient(client jobInserter) *WorkflowService
```

The first is never used (only `NewWorkflowServiceWithClient` is called from `server.go:98`). The generics constraint forces the duplication. Remove `NewWorkflowService` and rename `NewWorkflowServiceWithClient` to `NewWorkflowService`.

---

## 6. Server Struct Is a God Object

`Server` in `server.go:22-31` holds 8 fields and every handler is a method on it. This couples everything together. Consider splitting into smaller router groups:

```
server.go         — wire middleware, attach sub-routers
router_health.go  — health endpoint registration + handler
router_eval.go    — eval route group registration + handler
router_workflow.go — workflow route group registration + handler
router_chat.go    — chat route group registration + handler
```

Each sub-router could be a standalone struct that implements `http.Handler` or a registration function.

---

## 7. `db.Connect()` Ignores `config.Config`

`internal/db/db.go:15` reads `DATABASE_URL` directly from env, duplicating the default value (which is also in `config.go:47`). Meanwhile `config.Load()` already parsed `DatabaseURL`.

**Fix:** Change `db.Connect()` to accept the DSN as a parameter, and pass `cfg.DatabaseURL` from the caller.

---

## 8. `NewChatService()` Ignores `config.Config`

`service_chat.go:33-38` bypasses the config object entirely, calling `config.EnvOrDefault()` directly. It also doesn't use `config.ResolveProviderConfig()`, instead implementing its own ad-hoc fallback logic.

**Fix:** Accept `*config.Config` (and the server's `qstore.VectorStore`) as parameters.

---

## 9. Hardcoded 60s Timeout Middleware

`server.go:49` hardcodes `Timeout(60 * time.Second)`. This should be configurable via config (e.g., `API_REQUEST_TIMEOUT` env var). The stream endpoint in particular may need a longer timeout.

---

## 10. `respondJSON` Silently Swallows Write Errors

`response.go:21` — `json.NewEncoder(w).Encode(...)` — the error return is discarded. If the client disconnects mid-write, this is silently ignored. At minimum, log the error.

---

## 11. Artifact Handler Uses Relative Path

`handler_artifact.go:14` — `baseDir := "artifacts"` — this is relative to the server's working directory. If the process is started from elsewhere (e.g., Docker with a different workdir), it breaks. Use a config value or make it an absolute path resolved at startup.

---

## 12. CORS Middleware Unnecessary

`middleware.go:76-87` — the entire CORS middleware is unused in the deployment model. All traffic arrives through nginx on the same Docker network — no external browser clients, no cross-origin requests.

**Action:** Remove `CORS` from the middleware stack and delete the function. This eliminates one middleware wrapper per request and reduces the attack surface (no `Access-Control-Allow-Origin: *` leaking to internal services).

---

## 13. Qdrant gRPC Client: No Connection Pooling Needed

The Qdrant Go client wraps a single gRPC connection (`qdrant.NewGrpcClient`). gRPC connections are designed for multiplexing — a single long-lived client handles concurrent requests, keepalive, and transparent reconnection internally. There is no connection-pool API in the library, and one is not needed.

**What matters:** Sharing the one client across all consumers. Currently the `Server` creates one client (`server.go:39`) and `ChatService` creates its own second client (`service_chat.go:45`). The fix for item #1 (pass the server's `qstore.VectorStore` into `ChatService`) resolves the only real problem here. No pooling infrastructure is necessary.

---

## 14. Timeout Middleware Doesn't Write Error Response

`middleware.go:66-73` cancels the context but does not write an HTTP error. If the handler doesn't check `ctx.Done()`, the request continues silently. For handlers that do check, they'd get `context.Canceled` which they may or may not handle gracefully.

**Fix:** Either use chi's built-in `Timeout` middleware (which writes a 503), or wrap the handler response with a custom `http.ResponseWriter` that tracks context cancellation.

---

## 15. No Request Body Size Limit

All handlers that accept JSON bodies (`json.NewDecoder(r.Body)`) have no protection against large payloads. Add `http.MaxBytesReader` to prevent abuse.

---

## 16. `evalCompareHandler` Wastes SQL Work

`handler_eval.go:49` calls `evalSvc.GetRun()` with `limit=0, offset=0`, which causes SQL queries (`LIMIT 0 OFFSET 0`) and result scanning for each comparison run, even though only `.RunSummary` is used. Add a `GetRunSummary(ctx, id)` method that skips the questions query entirely.

---

## 17. `parseDocTimeout` Silently Returns 0 on Bad Input

`service_workflow.go:16-25` — if `time.ParseDuration` fails, returns 0 with no log. This means a user mistake in `doc_timeout` silently becomes "no timeout." Log the error or return it.

---

## 18. `Recovery` Middleware Missing Request Context

`middleware.go:58` logs the panic but doesn't include the request ID or method/path in the log entry. Add structured context to the panic log.

---

## 19. Chat Memory Capacity Hardcoded

`service_chat.go:62` — `memory.NewRingBuffer(10)`. The ring buffer size (max conversation turns) should be configurable, not hardcoded.

---

## 20. Test Boilerplate

`handler_workflow_test.go` repeats the pattern:

```go
s := &Server{
    workflows: &WorkflowService{
        client: &mockJobClient{...},
    },
}
```

in every test. A `testServer(t, opts...)` helper would reduce duplication significantly.

Similarly, the `handler_chat_stream_test.go` constructs `ChatService` manually. Tests within `package api` already export nothing but these patterns create lots of noise.

---

## 21. `evalCompareHandler` Doesn't Paginate

If a user passes `?compare_to=A&compare_to=B&compare_to=C...` for many IDs, each gets a full `GetRun` query (with questions). There's no limit on `?compare_to` values.

---

## Summary by Impact

| Priority   | Issue                                               | Effort | Impact                       |
| ---------- | --------------------------------------------------- | ------ | ---------------------------- |
| **High**   | Duplicate Qdrant connection                         | Small  | Reliability, resource waste  |
| **High**   | RAG logic duplication (chat vs stream)              | Medium | Maintainability, bug surface |
| **High**   | ChatService ignores config                          | Small  | Consistency, correctness     |
| **High**   | No request body size limit                          | Small  | Security                     |
| **High**   | db.Connect ignores config.DatabaseURL               | Tiny   | Correctness                  |
| **High**   | Use RFC 9457 Problem Details for errors             | Medium | Standards compliance, gateway compatibility |
| **Medium** | evalCompare N+1 queries                             | Small  | Performance                  |
| **Medium** | Timeout middleware doesn't write error              | Small  | UX                           |
| **Medium** | ChatRequest has no Validate()                       | Tiny   | Consistency                  |
| **Medium** | Server God object                                   | Medium | Testability                  |
| **Medium** | Artifact path is relative                           | Tiny   | Reliability                  |
| **Medium** | Hardcoded timeout/memory                            | Small  | Configurability              |
| **Medium** | WorkflowService dual ctors                          | Tiny   | Cleanliness                  |
| **Medium** | evalCompare wasteful SQL                            | Small  | Performance                  |
| **High**   | CORS middleware unnecessary — remove                | Tiny   | Security, surface area       |
| **Low**    | respondJSON swallows error                          | Tiny   | Debugging                    |
| **Low**    | parseDocTimeout silent                              | Tiny   | UX                           |
| **Low**    | Recovery missing context                            | Tiny   | Debugging                    |
| **Low**    | Test boilerplate                                    | Medium | Dev experience               |
| **Info**   | Qdrant gRPC: no pooling needed, share the instance  | N/A    | Clarification                |
