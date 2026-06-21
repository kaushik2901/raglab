# Medium-Severity Issues — Fix for Operational Maturity

---

## 1. No Per-Query Timeout on Database and Qdrant Calls

**Files:** `internal/api/service_eval.go`, `internal/eval/store.go`, `internal/store/qdrant.go`

**Why:** SQL queries and Qdrant gRPC calls use the caller's context but no specific query-level timeout is configured. If the database or Qdrant becomes slow or unresponsive (connection pool exhaustion, long-running query, network partition), a single request can block indefinitely. The `TimeoutMiddleware` on the HTTP layer may cancel the request, but the goroutine serving the request could still be blocked inside the database/sdriver call with no way to interrupt it (depending on driver implementation). This leads to goroutine leaks under load.

**Fix:** Set context timeouts at the call site for each database and Qdrant operation. Use a configurable per-query timeout (default 30s):

```go
ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
defer cancel()
rows, err := db.Query(ctx, "SELECT ...")
```

For Qdrant, pass the timeout context to each gRPC call:

```go
ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()
resp, err := client.Search(ctx, req)
```

---

## 2. No `.dockerignore` File

**File:** Missing (should be at project root)

**Why:** Without a `.dockerignore` file, the entire project directory (including `.git`, `node_modules` if any, test fixtures, CI config, documentation, and the local `bin/` directory) is sent to the Docker daemon as build context. This dramatically increases build times and consumes network bandwidth for remote Docker builds. It also risks including secrets (`.env`, SSH keys, local configs) in the Docker build context, which end up in the image history.

**Fix:** Create `.dockerignore` at the project root:

```
.git/
.gitignore
*.md
*.log
.env
.env.*
bin/
web/
testdata/
*.test
docker-compose*.yml
```

---

## 3. No `X-Request-ID` Propagation to Downstream Logging

**Files:** `internal/workflow/index_worker.go`, `internal/workflow/eval_worker.go`, `internal/workflow/preprocess_worker.go`

**Why:** The HTTP middleware generates a `X-Request-ID` and attaches it to the request context, but this ID is not propagated into River worker logs. When debugging a production issue, operators see request IDs in the API access logs but cannot correlate them with worker log entries. A single user request (e.g. triggering an eval run) creates multiple River jobs that log independently. Without correlation IDs, tracing the full lifecycle of a request requires manual timestamp matching across log streams.

**Fix:** Propagate the request ID (or a derived job ID) into River job args as a `CorrelationID` field. Workers should include this in all structured log output:

```go
slog.Info("processing document", "correlation_id", args.CorrelationID, "doc", docPath)
```

For API-initiated jobs, extract the request ID from the HTTP context and pass it to the River insert call.

---

## 4. No Audit Logging for Destructive Operations

**Files:** All delete handlers (`router_artifact.go`, `router_dataset.go`, `router_eval.go`)

**Why:** Delete operations (delete artifact, delete dataset, delete eval run, delete collection) are not logged to an audit trail. In production:
- An operator or attacker who deletes critical data leaves no trace.
- Compliance requirements (SOC2, HIPAA, GDPR) often mandate audit logs for data deletion.
- Incident response has no information about who deleted what and when.

**Fix:** Add structured audit log entries before each destructive operation. Include timestamp, actor identity (when auth is added), resource type, resource identifier, and request metadata:

```go
slog.Warn("audit: delete resource",
    "resource_type", "artifact",
    "resource_id", tag,
    "actor", getUser(r.Context()),
    "ip", r.RemoteAddr,
    "request_id", GetRequestID(r.Context()),
)
```

---

## 5. Migrations Run Every Startup with No Version Tracking

**File:** `internal/db/migrate.go:20-31`

**Why:** SQL migrations are applied unconditionally on every startup via `rivermigrate.Migrate()`. While the River migration library tracks which migrations have been applied, the custom SQL file (`001_initial.sql`) re-runs `CREATE TABLE IF NOT EXISTS` and `ALTER TABLE ... DROP COLUMN IF EXISTS` every time. This means:
- Startup time includes re-applying DDL statements unnecessarily.
- The `ALTER TABLE DROP COLUMN IF EXISTS` in the initial migration (which was added to fix a previous schema issue) runs on every startup — this is a sign the migration was not properly versioned.
- There is no way to run migrations independently (e.g. as a separate init container or migration job).

**Fix:** Split the schema into proper versioned migration files (001_initial.sql, 002_add_column.sql, etc.). Remove the `DROP COLUMN IF EXISTS` from the initial migration. Run migrations as a separate step (init container or startup command) rather than embedding them in the application startup:

```go
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
    migrator, err := rivermigrate.New(pool, &rivermigrate.Config{})
    // ...
    _, err = migrator.MigrateTx(ctx, nil, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
    return err
}
```

Then call this from an init container or start-up script, not from every server/worker start.

---

## 6. Config Validation Allows Empty API Keys for Required Providers

**File:** `internal/config/config.go:93-103`

**Why:** The `Validate()` method checks that `LLMApiKey` is not empty, but this check is applied *before* provider resolution. The resolved provider uses environment variables like `OPENAI_API_KEY`, `GEMINI_API_KEY`, etc. If a user sets a provider (e.g. `openai`) but sets the API key via `OPENAI_API_KEY` (which maps to `LLMApiKey`), the validation passes. But if the user sets both `OPENAI_API_KEY` and `GEMINI_API_KEY`, the wrong one may be used silently. More critically, if `LLM_PROVIDER=openai` but `OPENAI_API_KEY` is empty, the validation passes (because `LLMApiKey` is also empty) and the embedder/generator will fail with a confusing authentication error at runtime.

**Fix:** After provider resolution, validate that the resolved API key is non-empty for providers that require one (all except `lmstudio`):

```go
switch cfg.Provider {
case "openai", "openrouter":
    if cfg.LLMApiKey == "" {
        return fmt.Errorf("LLM_API_KEY must be set for provider %s", cfg.Provider)
    }
case "gemini":
    if os.Getenv("GEMINI_API_KEY") == "" {
        return fmt.Errorf("GEMINI_API_KEY must be set for provider gemini")
    }
case "lmstudio":
    // No API key required
}
```

---

## 7. `init()` Functions for Strategy Registration — Side Effects on Import

**Files:** `internal/chunker/fixed.go:105`, `internal/chunker/recursive.go:37`

**Why:** Strategy registration uses Go `init()` functions to call `RegisterChunker("fixed", ...)` and `RegisterParser("markdown", ...)`. These run at package import time, even if the package is imported only for its types. This means:
- Importing the package for any reason (even just for a type definition in a test) triggers side effects that modify global registries.
- Order of registration is non-deterministic (depends on import order).
- It is impossible to register alternative strategies or change registration order without modifying the source.

**Fix:** Replace `init()` functions with explicit registration calls in `main.go` or a dedicated `init.go` that is explicitly called:

```go
// In cmd/api/main.go or cmd/workerd/main.go
func init() {
    chunker.Register("fixed", chunker.NewFixedChunker)
    chunker.Register("recursive", chunker.NewRecursiveChunker)
    parser.Register("markdown", parser.NewMarkdownParser)
}
```

Or use a registry pattern that allows lazy registration and override.

---

## 8. `golang.org/x/net v0.55.0` Contains Known CVEs

**File:** `go.mod:18`

**Why:** The project depends on `golang.org/x/net v0.55.0`, which has known security vulnerabilities (CVE-2024-24791, CVE-2024-24790, etc.) related to HTTP/2 and proxy handling. While this is an indirect dependency, it is pulled into the compiled binary. Attackers who can send crafted HTTP/2 frames to the service or trigger specific proxy behaviors can exploit these vulnerabilities.

**Fix:** Update `golang.org/x/net` to the latest version (`v0.36.0` or newer) by running:

```sh
go get golang.org/x/net@latest
go mod tidy
```

Then rebuild. Verify no breaking changes in the updated dependency.

---

## 9. No Structured JSON Log Format in Workerd

**File:** `cmd/workerd/main.go`

**Why:** The API server uses `slog.NewTextHandler(os.Stdout, nil)` (human-readable text), and the workerd uses the default which is also text. In production, log aggregation tools (Datadog, Loki, ELK, CloudWatch) expect structured JSON logs for reliable parsing, filtering, and alerting. Text logs require fragile regex parsing and miss fields that structured logging provides. The default log level is `info`, meaning debug-level logs are discarded even when troubleshooting production issues.

**Fix:** Use JSON log handler and allow log level to be configurable via environment variable:

```go
level := slog.LevelInfo
if l := os.Getenv("LOG_LEVEL"); l != "" {
    if parsed, err := slog.ParseLevel(l); err == nil {
        level = parsed
    }
}
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: level,
})))
```

---

## 10. No Input Size Limits on JSON Request Bodies

**Files:** `internal/api/router_chat.go:27`, `internal/api/router_eval.go`

**Why:** HTTP handlers that accept JSON bodies do not enforce size limits on the request body beyond the general MaxBytesReader in `server.go`. Chat requests, eval job submissions, and other mutation endpoints accept unbounded payloads. A large JSON body (e.g. an eval request with thousands of questions, or a chat request with massive history) consumes server memory proportional to input size, enabling resource-exhaustion attacks.

**Fix:** Apply `http.MaxBytesReader` per-handler with endpoint-appropriate limits:

```go
const MaxChatBody = 1 << 20 // 1 MB
r.Body = http.MaxBytesReader(w, r.Body, MaxChatBody)
```

Or use a middleware that enforces `Content-Length` validation before parsing.

---

## 11. `os.Kill` in Signal Handling

**File:** `cmd/workerd/main.go:14,20`

**Why:** The signal handler registers for `os.Kill` in addition to `syscall.SIGTERM` and `syscall.SIGINT`. However, `os.Kill` cannot be caught by a Go program (the runtime always terminates). This line has no effect and misleads readers into thinking SIGKILL is handled gracefully. If the intent is to also handle `SIGHUP` (config reload) or `SIGQUIT` (stack trace), those should be used instead.

**Fix:** Remove `os.Kill` from the signal list:

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
```

---

## 12. Database Pool Closed Without Awaiting Active Queries

**File:** `cmd/workerd/main.go:79`

**Why:** The database pool is closed immediately after `riverClient.Stop()` returns, without checking if there are still active queries. If River's stop completes but the database pool still has in-flight queries (due to driver buffering or timing), closing the pool prematurely terminates those queries and can leave transactions in an indeterminate state.

**Fix:** After River client stops, drain the pool with a timeout before closing:

```go
// Allow existing queries to finish
time.Sleep(1 * time.Second) // brief drain period
pool.Close()
```

Or check `pool.AcquireAllIdle(ctx)` to wait for active connections to become idle before closing.

---

## 13. No CSRF Protection on Form Uploads

**File:** `internal/api/router_dataset.go`

**Why:** The dataset upload handler accepts multipart form submissions with no CSRF (Cross-Site Request Forgery) protection. Once auth is added, a logged-in user visiting a malicious site could trigger a dataset upload by submitting a form cross-origin. The browser automatically includes cookies, making the upload appear legitimate. This allows data injection or overwrite attacks.

**Fix:** For a server-to-server API (no browser cookies), this is low risk once CORS is properly configured with credential checks. If the API will serve browser clients with cookie-based auth, implement CSRF tokens (double-submit cookie pattern or `SameSite=Strict` cookies). For now, the most important protection is proper CORS + auth.

---

## 14. No Pod Shutdown Delay for Workerd

**File:** `cmd/workerd/main.go`

**Why:** When running in Kubernetes or Nomad, the orchestrator sends SIGTERM and waits for the container to exit. If the process exits immediately (within a few seconds), the orchestrator may have already removed the pod from service discovery, but in-flight connections and jobs may still be active. Without a `preStop` lifecycle hook or a minimum shutdown delay, hard termination (SIGKILL) is sent by the orchestrator after its grace period if the process exits too quickly.

**Fix:** In production deployment manifests, add a `preStop` hook with a sleep:

```yaml
lifecycle:
  preStop:
    exec:
      command: ["sh", "-c", "sleep 15"]
```

This gives load balancers time to drain connections and the workerd time to finish in-flight jobs before SIGTERM is delivered.

---

## 15. Default Qdrant URL Hardcoded in Workers

**Files:** `internal/workflow/index_worker.go:136`, `internal/workflow/eval_worker.go:389`

**Why:** The Qdrant gRPC URL defaults to `http://localhost:6334` (note: `http://` for gRPC, which should be `grpc://` or plain host:port). This is hardcoded in the worker code rather than read from env/config. If Qdrant runs on a different host (as it would in any non-local deployment), workers silently connect to a non-existent Qdrant on localhost, failing with confusing connection errors. The config system already has a `QdrantURL` field, but workers don't use it.

**Fix:** Pass the Qdrant URL through job args (consistent with the project convention of no `*config.Config` in workers):

```go
type IndexArgs struct {
    // ... existing fields ...
    QdrantURL string `json:"qdrant_url"`
}
```

Set it from config when inserting jobs via the API service.

---

## 16. No Log Level Configuration in Workerd

**File:** `cmd/workerd/main.go`

**Why:** The workerd uses the default `slog` handler with no explicit level configuration. Logs default to `slog.LevelInfo`. In production debugging scenarios (e.g. diagnosing a failing index job), operators need to temporarily enable `debug` logging without rebuilding the binary. There is no mechanism to change the log level at runtime.

**Fix:** Read `LOG_LEVEL` from environment (same as API server) at startup:

```go
if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
    if parsed, err := slog.ParseLevel(lvl); err == nil {
        slog.SetLogLoggerLevel(parsed)
    }
}
```

---

## 17. `make.cmd` Is Windows-Only — No Linux/Mac Equivalent

**File:** `make.cmd`

**Why:** The project only has a Windows batch file (`make.cmd`) for build/test commands. Linux and macOS developers must manually construct the `go build` and `docker compose` commands or read AGENTS.md for the correct invocations. This is a friction point for onboarding and CI setup on non-Windows platforms.

**Fix:** Create a POSIX `Makefile` with the same targets:

```makefile
.PHONY: build-api build-workerd test

build-api:
	go build -o bin/api ./cmd/api

build-workerd:
	go build -o bin/workerd ./cmd/workerd

test:
	go test ./...

up:
	docker compose up -d
```

---

## 18. `DocTimeout` of Zero Disables Per-Document Timeout

**File:** `internal/workflow/index_worker.go:206`

**Why:** Each document in the index pipeline is assigned a context timeout via `DocTimeout`, but the default value is `0` — which means `context.WithTimeout(ctx, 0)` creates a context that immediately expires (or more precisely, `0` means no timeout in Go's context package, so it inherits the parent context with no additional bound). A single large document that takes hours to parse/chunk/embed blocks the entire index worker queue. There is no upper bound on per-document processing time.

**Fix:** Set a sensible default in the job args (e.g. 10 minutes per document) and enforce it:

```go
const DefaultDocTimeout = 10 * time.Minute

docTimeout := args.DocTimeout
if docTimeout <= 0 {
    docTimeout = DefaultDocTimeout
}
ctx, cancel := context.WithTimeout(ctx, docTimeout)
```

---

## 19. Hardcoded `5*time.Minute` Batch Embed Timeout

**File:** `internal/workflow/index_worker.go:53`

**Why:** The batch embedding timeout is hardcoded to 5 minutes. For large batches (e.g. thousands of chunks) or slow embedding providers (e.g. rate-limited free tier), 5 minutes may be too short, causing batch embedding failures that require manual retry. Conversely, for fast providers, 5 minutes may be unnecessarily long, delaying error detection.

**Fix:** Make the timeout configurable via job args with the 5-minute default:

```go
type IndexArgs struct {
    EmbedBatchTimeout time.Duration `json:"embed_batch_timeout"`
}
```

---

## 20. `ParseRetryAfter` Is Dead Code

**File:** `internal/config/normalize.go:25`

**Why:** The `ParseRetryAfter` function is defined but never called anywhere in the codebase. This is dead code that adds maintenance overhead and confuses readers. It may have been intended for rate-limit header parsing but was never integrated.

**Fix:** Either remove it or implement it in the rate-limiting retry logic where `Retry-After` headers from LLM providers are respected:

```go
// If this function is needed, use it in embedder/generator retry logic:
if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
    wait := ParseRetryAfter(retryAfter)
    // ...
}
```

Otherwise, delete it.

---

## 21. `FloatEnvOrDefault` Not Used Outside Tests

**File:** `internal/config/config.go:182`

**Why:** The helper `FloatEnvOrDefault` is only used in tests. This is either dead code or a sign that a config value that should be floating-point is not being read from the environment. It adds unnecessary exported API surface.

**Fix:** Either use it for a real config field (e.g. a fraction or threshold) or remove it to keep the public API minimal.

---

## 22. Chat Migration Runs Even If Chat Feature Is Not Used

**File:** `internal/db/migrate.go:23`

**Why:** The `chat_messages` table migration runs on every startup unconditionally. If the chat feature is not deployed, this creates an unnecessary table and wastes startup time. More importantly, it couples schema requirements for optional features to the core application lifecycle.

**Fix:** Conditionally run chat-specific migrations only when the chat service is initialized, or move it to a separate migration file that is always run but documents that it's for the chat feature.

---

## 23. Include Resolution Allows Arbitrary File Reads Within Repo

**File:** `internal/preprocessor/includes.go:30-33`

**Why:** The Hugo include resolver reads files from the cloned repository based on the include path. While constrained to the repo root, there is no validation that the resolved path is not a symlink pointing outside the repo, or that the file type is appropriate. A malicious preprocessed document could include arbitrary files within the repo (e.g. `.env`, secrets, config files), leaking them into the processed output and potentially into the vector store.

**Fix:** After resolving the include path, verify the resolved file is a regular file (not a symlink) and has an allowed extension (.md, .html, etc.):

```go
info, err := os.Stat(resolvedPath)
if err != nil || info.Mode()&os.ModeSymlink != 0 {
    return "", fmt.Errorf("invalid include: %s", includePath)
}
```
