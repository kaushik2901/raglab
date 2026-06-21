# Critical Issues — Must Fix Before Production

## 1. No CORS Middleware

**Why:** Cross-Origin Resource Sharing is not configured. Any browser-based frontend will be blocked by the same-origin policy. Without CORS headers, requests from a web UI hosted at a different origin (e.g. `app.example.com` calling `api.example.com`) will fail with no clear error to the user. In production where the API and frontend are served from different origins, the app is completely non-functional for browser clients.

**Fix:** Add a CORS middleware using `github.com/go-chi/cors` (already using `chi` router). Configure allowed origins from environment variables (`CORS_ALLOWED_ORIGINS`), allow common methods (`GET, POST, PUT, DELETE, OPTIONS`), and handle preflight `OPTIONS` requests. Apply the middleware globally in `server.go`:

```go
import "github.com/go-chi/cors"

corsMiddleware := cors.Handler(cors.Options{
    AllowedOrigins:   strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ","),
    AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
    AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
    ExposedHeaders:   []string{"Link"},
    AllowCredentials: true,
    MaxAge:           300,
})
r.Use(corsMiddleware)
```

---

## 2. No API-Level Rate Limiting

**Why:** There is no request rate limiting at the API gateway level. The only rate limiting is at the LLM provider layer (token bucket for embedding/generation API calls). An attacker or misbehaving client can flood `/api/v1/chat`, `/api/v1/eval`, or any endpoint with unlimited requests, causing resource exhaustion on the server (CPU, memory, PG connections, Qdrant connections) and unbounded costs from LLM provider APIs. This is a denial-of-service vector.

**Fix:** Add a per-IP or per-API-key rate limiter middleware. Use a sliding window or token bucket approach. Store state in-memory (for single-instance) or Redis (for multi-instance). Apply at least a global limit (e.g. 100 req/s) and a per-endpoint limit for expensive operations like chat (e.g. 10 req/s). Example using a simple in-memory token bucket:

```go
func RateLimit(rate int, burst int) func(http.Handler) http.Handler {
    limiter := rate.NewLimiter(rate.Limit(rate), burst)
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if !limiter.Allow() {
                http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

---

## 3. No Prometheus Metrics or OpenTelemetry Tracing

**Why:** There is zero observability in production. No request latency histograms, no error rate counters, no database connection pool metrics, no circuit breaker state metrics, no business-level metrics (documents indexed, questions evaluated, tokens consumed). When something goes wrong in production, operators have no data to diagnose the issue. There is no way to set up alerts (e.g. p99 latency > 5s, error rate > 1%). There is also no distributed tracing, making it impossible to trace a single request across the API → database → Qdrant → LLM provider call chain.

**Fix:** Add a `/metrics` endpoint exposing Prometheus metrics via `promhttp.Handler`. Instrument the chi router with `go-chi/prometheus` middleware for automatic request duration, status code, and counter metrics. Add business metrics (gauges, counters, histograms) at key points:
- Document/chunk processing counters in the index worker
- Question evaluation counters and latency in the eval worker
- Circuit breaker state gauges
- Database pool stats via `db.Stats()`

For tracing, add OpenTelemetry instrumentation: wrap the HTTP server with `otelhttp.NewHandler`, add span creation in database calls and Qdrant calls. Export to a configurable OTLP endpoint.

---

## 4. No `http.Server` Read/Write/Idle Timeouts

**File:** `internal/api/server.go:110`

**Why:** The `http.Server` is created with zero values for `ReadTimeout`, `WriteTimeout`, `ReadHeaderTimeout`, and `IdleTimeout`. This means:
- A slow client can send headers one byte at a time, holding a connection open indefinitely (resource exhaustion).
- A handler that hangs will hold the connection forever (no write deadline).
- Idle keep-alive connections are never recycled.

In production, this leads to connection leaks, goroutine leaks, and eventual service degradation under load.

**Fix:** Set explicit timeouts on `http.Server`:

```go
&http.Server{
    Addr:              addr,
    Handler:           r,
    ReadTimeout:       10 * time.Second,
    ReadHeaderTimeout: 5 * time.Second,
    WriteTimeout:      60 * time.Second,
    IdleTimeout:       120 * time.Second,
}
```

Adjust values based on expected longest request (e.g. streaming chat needs a longer `WriteTimeout`; consider per-handler deadlines instead).

---

## 5. Timeout Middleware Logic Bug

**File:** `internal/api/middleware.go:80-99`

**Why:** The timeout middleware checks the context deadline *after* `next.ServeHTTP(w, r)` returns. By that point, the handler has already completed (or is still blocking). The middleware writes a 504 response after the handler returns, but:
- The handler may have already written a partial response (leading to a `superfluous response.WriteHeader` log).
- The middleware does not actually enforce that the handler stops processing. The underlying goroutine continues running until it checks the context itself.
- The deadline is checked using `select` which does not prevent the handler from consuming resources.

This middleware provides a false sense of timeout protection while actually doing nothing to bound handler execution time.

**Fix:** Replace with a proper timeout middleware that creates a context with deadline, uses a `responseWriter` wrapper to detect writes, and on timeout cancels the context and writes 504 before the handler completes. The standard pattern:

```go
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx, cancel := context.WithTimeout(r.Context(), timeout)
            defer cancel()
            done := make(chan struct{})
            tw := &timeoutWriter{ResponseWriter: w, wrote: make(chan struct{})}
            go func() {
                next.ServeHTTP(tw, r.WithContext(ctx))
                close(done)
            }()
            select {
            case <-done:
                return
            case <-ctx.Done():
                tw.mu.Lock()
                if !tw.wroteHeader {
                    w.WriteHeader(http.StatusGatewayTimeout)
                }
                tw.mu.Unlock()
            }
        })
    }
}
```

Better yet, use `http.TimeoutHandler` from the standard library.

---

## 6. Path Traversal in Artifact Download

**File:** `internal/api/router_artifact.go:89-94`

**Why:** The artifact handler constructs file paths by joining user-supplied `artifactType`, `uniq`, `tag`, and `subpath` parameters using `filepath.Join`. While there is a basic check that the resolved path starts with the expected base directory, this check is insufficient. If the base path itself contains symlinks, or if `filepath.EvalSymlinks` is not called before the prefix check, an attacker can craft inputs like `../../../etc/passwd` to read arbitrary files on the server filesystem. Additionally, `http.ServeFile` does not prevent symlink traversal out of the intended directory.

**Fix:** Use `filepath.Clean` + `filepath.EvalSymlinks` on the resolved path, then verify it actually starts with the intended base directory. Reject requests where the resolved path escapes. Consider using `http.ServeContent` with an `os.File` opened after path validation instead of `http.ServeFile`:

```go
base := filepath.Join(artifactsDir, artifactType, tag)
target := filepath.Join(base, subpath)
cleanTarget, err := filepath.EvalSymlinks(target)
if err != nil || !strings.HasPrefix(cleanTarget, filepath.Clean(base)+string(os.PathSeparator)) {
    http.Error(w, "invalid path", http.StatusBadRequest)
    return
}
```

---

## 7. Dataset Upload Path Injection from Filename

**File:** `internal/api/router_dataset.go:47-62`

**Why:** The dataset upload handler uses the filename from the `multipart.FileHeader.Filename` field (supplied by the client in the `Content-Disposition` header) to construct the destination file path. A malicious client can set `filename` to `../../etc/cronjob` or include path separators to overwrite arbitrary files on the server. There is no sanitization, no basename extraction, and no validation that the filename is safe.

**Fix:** Strip the basename from the user-supplied filename and generate a safe storage name. Never trust user-supplied filenames. Sanitize by removing path separators and using only the basename:

```go
safeFilename := filepath.Base(header.Filename)
// Or generate a UUID-based filename to avoid collision/overwrite entirely
destPath := filepath.Join(uploadDir, fmt.Sprintf("%s_%s", uuid.New().String(), safeFilename))
```

Also apply `MaxBytesReader` *before* `r.ParseMultipartForm` (currently applied after, which means the limit is ineffective for the initial read).

---

## 8. No Container Restart Policies or Healthchecks

**File:** `docker-compose.yml`

**Why:** No service in `docker-compose.yml` specifies `restart:` or `healthcheck:`. If any container crashes (OOM, segfault, connection exhaustion), it stays dead until manually restarted. This means:
- A transient Qdrant crash takes down the entire pipeline.
- The API server or workerd crashing silently halts all processing with no recovery.
- Orchestrators like Docker Swarm or Nomad will also not auto-recover.

Additionally, the API and workerd containers have no healthcheck, so load balancers and orchestrators cannot determine if the service is actually ready to handle traffic.

**Fix:** Add `restart: unless-stopped` to all services. Add healthchecks for each service:

```yaml
restart: unless-stopped
healthcheck:
  test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/health" ]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 10s
```

For Qdrant, add a healthcheck using its `/healthz` endpoint. For Postgres, use `pg_isready`. For workerd, a simple TCP check on its port.

---

## 9. No Container Memory/CPU Limits

**File:** `docker-compose.yml`

**Why:** No memory or CPU limits are set on any container. In production, a single runaway container (e.g. memory leak in the index worker processing a large document) can consume all host resources and starve other containers. This compromises the entire deployment and can lead to OOM kills by the kernel, affecting unrelated services.

**Fix:** Set conservative resource limits on each service:

```yaml
deploy:
  resources:
    limits:
      cpus: '2'
      memory: 2G
    reservations:
      cpus: '0.5'
      memory: 512M
```

For `docker compose` (non-swarm), use Docker's native `--memory`/`--cpus` via `mem_limit`/`cpus` in the service definition:

```yaml
mem_limit: 2g
cpus: '2'
```
