# API Server Implementation Plan

Build a production-quality REST API server to replace CLI interaction. The API is a separate process from `workerd` — it inserts River jobs and polls their status, but never runs workers itself.

---

## Table of Contents

1. [Package Structure](#package-structure)
2. [Sub-Phase Plan](#sub-phase-plan)
3. [Endpoint Reference](#endpoint-reference)
4. [Middleware Stack](#middleware-stack)
5. [Error Handling](#error-handling)
6. [River Integration](#river-integration)
7. [Chat / SSE](#chat--sse)
8. [Testing Strategy](#testing-strategy)
9. [Docker Compose](#docker-compose)

---

## Package Structure

```
cmd/api/main.go
internal/api/
  server.go           # Server struct, chi, routes, ListenAndServe, graceful shutdown
  middleware.go        # RequestID, StructuredLog, Recovery, Timeout, CORS
  response.go          # respondJSON, respondError, respondNoContent
  types.go             # Request/response structs + validation helpers

  handler/
    health.go          # GET /health
    workflow.go        # POST /workflows/preprocess, /index, /eval + GET /workflows/:id
    chat.go            # POST /chat (SSE)
    eval.go            # GET /eval/runs, /eval/runs/:id, /eval/runs/:id/compare
    artifact.go        # GET /artifacts

  service/
    workflow.go        # Insert River jobs, JobGet wrapper
    chat.go            # Embed → retrieve → build context → generate → memory
    eval.go            # List runs, get results, compare across runs
    artifact.go        # Walk filesystem under artifacts/
```

No `handler/` and `service/` sub-directories — just `handler_health.go`, `service_workflow.go` etc. flat in `internal/api/` to keep imports simple.

---

## Sub-Phase Plan

### Phase 1: Skeleton

**Files:** `cmd/api/main.go`, `internal/api/server.go`

**Goal:** Bootable binary that starts a chi router and shuts down gracefully.

**`cmd/api/main.go`:**
```go
package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
    defer cancel()

    if err := run(ctx); err != nil {
        slog.Error("fatal", "err", err)
        os.Exit(1)
    }
}

func run(ctx context.Context) error {
    cfg, err := config.Load()
    if err != nil {
        return err
    }
    // ... slog setup from config.LogLevel ...

    srv, err := api.New(cfg)
    if err != nil {
        return err
    }
    return srv.ListenAndServe(ctx)
}
```

**`internal/api/server.go`:**
```go
package api

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"

    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
)

type Server struct {
    cfg    *config.Config
    router *chi.Mux
    http   *http.Server
}

func New(cfg *config.Config) (*Server, error) {
    r := chi.NewRouter()
    return &Server{
        cfg:    cfg,
        router: r,
    }, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
    addr := fmt.Sprintf(":%s", config.EnvOrDefault("API_PORT", "8080"))
    s.http = &http.Server{Addr: addr, Handler: s.router}

    go func() {
        <-ctx.Done()
        shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        s.http.Shutdown(shutdown)
    }()

    slog.Info("api server starting", "addr", addr)
    if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        return err
    }
    return nil
}
```

**Deliverable:** `go build ./cmd/api` produces a binary that starts on `:8080` and responds 404 on all routes.

---

### Phase 2: Middleware + Response Helpers

**Files:** `internal/api/middleware.go`, `internal/api/response.go`

**Goal:** Structured request logging, panic recovery, request IDs, CORS, and consistent JSON error responses.

**`internal/api/response.go`:**
```go
package api

import (
    "encoding/json"
    "net/http"
)

type APIError struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}

type envelope struct {
    Data  any        `json:"data,omitempty"`
    Error *APIError  `json:"error,omitempty"`
}

func respondJSON(w http.ResponseWriter, status int, data any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(envelope{Data: data})
}

func respondError(w http.ResponseWriter, status int, code, msg string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(envelope{
        Error: &APIError{Code: code, Message: msg},
    })
}

func respondNoContent(w http.ResponseWriter) {
    w.WriteHeader(http.StatusNoContent)
}
```

Every response is wrapped in `{"data": ...}` on success or `{"error": {"code": "...", "message": "..."}}` on error. This gives consumers a consistent envelope to parse.

**`internal/api/middleware.go`:**
```go
package api

import (
    "log/slog"
    "net/http"
    "runtime/debug"
    "time"

    "github.com/google/uuid"
)

func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.Header.Get("X-Request-ID")
        if id == "" {
            id = uuid.NewString()
        }
        w.Header().Set("X-Request-ID", id)
        ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

type ctxKey int
const ctxKeyRequestID ctxKey = iota

func StructuredLog(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        sw := &statusWriter{ResponseWriter: w, status: 200}
        next.ServeHTTP(sw, r)
        slog.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", sw.status,
            "duration", time.Since(start).String(),
            "request_id", r.Context().Value(ctxKeyRequestID),
        )
    })
}

type statusWriter struct {
    http.ResponseWriter
    status int
}

func (w *statusWriter) WriteHeader(status int) {
    w.status = status
    w.ResponseWriter.WriteHeader(status)
}

func Recovery(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                slog.Error("panic", "err", rec, "stack", string(debug.Stack()))
                respondError(w, 500, "INTERNAL_ERROR", "internal server error")
            }
        }()
        next.ServeHTTP(w, r)
    })
}

func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx, cancel := context.WithTimeout(r.Context(), timeout)
            defer cancel()
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

CORS middleware — allow all origins (dev mode):

```go
func CORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
        if r.Method == "OPTIONS" {
            w.WriteHeader(204)
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

**Wire middleware** in `server.go`:

```go
func New(cfg *config.Config) (*Server, error) {
    r := chi.NewRouter()
    r.Use(RequestID)
    r.Use(StructuredLog)
    r.Use(Recovery)
    r.Use(Timeout(60 * time.Second))
    r.Use(CORS)
    // routes added in later phases
    return &Server{cfg: cfg, router: r}, nil
}
```

---

### Phase 3: Health Endpoint

**Files:** `internal/api/handler_health.go`

**Goal:** `GET /health` pings Postgres and Qdrant, returns 200 or 503.

```go
package api

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
    services := map[string]string{}

    if err := s.pool.Ping(r.Context()); err != nil {
        services["postgres"] = "disconnected: " + err.Error()
    } else {
        services["postgres"] = "connected"
    }

    if err := s.qdrant.HealthCheck(r.Context()); err != nil {
        services["qdrant"] = "disconnected: " + err.Error()
    } else {
        services["qdrant"] = "connected"
    }

    allOK := true
    for _, status := range services {
        if status != "connected" {
            allOK = false
        }
    }

    if !allOK {
        respondJSON(w, http.StatusServiceUnavailable, map[string]any{
            "status":   "degraded",
            "version":  version,
            "services": services,
        })
        return
    }

    respondJSON(w, http.StatusOK, map[string]any{
        "status":   "ok",
        "version":  version,
        "services": services,
    })
}
```

**Requires** `pgxpool.Pool` and a Qdrant client in the `Server` struct. Update `New()` to accept them or lazy-init:

```go
type Server struct {
    cfg    *config.Config
    router *chi.Mux
    http   *http.Server
    pool   *pgxpool.Pool
    qdrant store.VectorStore
}

func New(cfg *config.Config) (*Server, error) {
    pool, err := db.Connect(context.Background())
    if err != nil {
        return nil, fmt.Errorf("connect postgres: %w", err)
    }
    // Qdrant connect
}
```

Add a `HealthCheck(ctx) error` method to `store.VectorStore` (currently missing — needs adding to the interface and `QdrantStore`):

```go
// internal/store/store.go
type VectorStore interface {
    Connect(ctx context.Context, dsn string) error
    EnsureCollection(ctx context.Context, name string, vectorSize int, distance string) error
    Store(ctx context.Context, collectionName string, chunks []types.DocumentChunk) error
    Search(ctx context.Context, collectionName string, queryVector []float32, topK int) ([]types.SearchResult, error)
    HealthCheck(ctx context.Context) error
    Close() error
}

// internal/store/qdrant.go
func (s *QdrantStore) HealthCheck(ctx context.Context) error {
    _, err := s.client.Collections().CollectionExists(ctx, &qdrant.CollectionExistsRequest{
        CollectionName: "_health_check",
    })
    return err
}
```

**Route registration:**
```go
r.Get("/health", s.healthHandler)
```

---

### Phase 4: Workflow Trigger Endpoints

**Files:** `internal/api/service_workflow.go`, `internal/api/handler_workflow.go`, `internal/api/types.go`

**Goal:** `POST /api/v1/workflows/preprocess`, `/index`, `/eval` — validate request, insert River job, return 202.

**`internal/api/types.go`:**
```go
package api

// PreprocessRequest
type PreprocessRequest struct {
    RepoURL     string   `json:"repo_url"`
    Tag         string   `json:"tag,omitempty"`
    IncludeDirs []string `json:"include_dirs,omitempty"`
}

type IndexRequest struct {
    InputTag           string `json:"input_tag"`
    Tag                string `json:"tag,omitempty"`
    ParserStrategy     string `json:"parser_strategy,omitempty"`
    ChunkStrategy      string `json:"chunk_strategy,omitempty"`
    ChunkSize          *int   `json:"chunk_size,omitempty"`
    ChunkOverlap       *int   `json:"chunk_overlap,omitempty"`
    EmbeddingProvider  string `json:"embedding_provider,omitempty"`
    EmbeddingModel     string `json:"embedding_model,omitempty"`
    BatchSize          *int   `json:"batch_size,omitempty"`
    IndexConcurrency   *int   `json:"index_concurrency,omitempty"`
    DocTimeout         string `json:"doc_timeout,omitempty"`
}

type EvalRequest struct {
    IndexTag          string   `json:"index_tag"`
    Tag               string   `json:"tag,omitempty"`
    QueryStrategy     string   `json:"query_strategy"`
    DatasetPath       string   `json:"dataset_path"`
    TopK              *int     `json:"top_k,omitempty"`
    Ks                []int    `json:"ks,omitempty"`
    LLMProvider       string   `json:"llm_provider,omitempty"`
    LLMModel          string   `json:"llm_model,omitempty"`
    EmbeddingProvider string   `json:"embedding_provider,omitempty"`
    EmbeddingModel    string   `json:"embedding_model,omitempty"`
    JudgeProvider     string   `json:"judge_provider,omitempty"`
    JudgeModel        string   `json:"judge_model,omitempty"`
    BatchSize         *int     `json:"batch_size,omitempty"`
    Workers           *int     `json:"workers,omitempty"`
}

// WorkflowResponse — returned by all workflow trigger endpoints
type WorkflowResponse struct {
    JobID     int64  `json:"job_id"`
    Tag       string `json:"tag"`
    State     string `json:"state"`
    CreatedAt string `json:"created_at"`
}
```

Validation function:
```go
// PreprocessRequest.Validate() error
// IndexRequest.Validate() error
// EvalRequest.Validate() error
```

**`internal/api/service_workflow.go`:**
```go
package api

import (
    "context"
    "fmt"
    "strconv"
    "time"

    "github.com/riverqueue/river"
    "github.com/riverqueue/river/rivertype"

    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/config"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/workflow"
)

type WorkflowService struct {
    client *river.Client[pgx.Tx]
}

func (s *WorkflowService) InsertPreprocess(ctx context.Context, req PreprocessRequest) (*WorkflowResponse, error) {
    tag := config.ResolveTag(req.Tag, "pre")
    result, err := s.client.Insert(ctx, &workflow.PreprocessArgs{
        Tag:         tag,
        RepoURL:     req.RepoURL,
        IncludeDirs: req.IncludeDirs,
    }, nil)
    if err != nil {
        return nil, fmt.Errorf("insert preprocess job: %w", err)
    }
    return jobToResponse(result.Job, tag), nil
}

// InsertIndex, InsertEval — same pattern with their respective args

func (s *WorkflowService) GetJob(ctx context.Context, id int64) (*JobStatusResponse, error) {
    row, err := s.client.JobGet(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("get job %d: %w", id, err)
    }
    return &JobStatusResponse{
        JobID:       row.ID,
        State:       jobStateString(row.State),
        AttemptedAt: row.AttemptedAt.Format(time.RFC3339),
        CompletedAt: row.FinalizedAt.Format(time.RFC3339),
        Errors:      formatErrors(row.Errors),
    }, nil
}

func jobStateString(s rivertype.JobState) string {
    switch s {
    case rivertype.JobStateAvailable:
        return "available"
    case rivertype.JobStateRunning:
        return "running"
    // ...
    }
}
```

**`internal/api/handler_workflow.go`:**
```go
package api

func (s *Server) preprocessHandler(w http.ResponseWriter, r *http.Request) {
    var req PreprocessRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, 400, "INVALID_JSON", "invalid request body")
        return
    }
    if err := req.Validate(); err != nil {
        respondError(w, 400, "INVALID_PARAMETER", err.Error())
        return
    }
    resp, err := s.workflows.InsertPreprocess(r.Context(), req)
    if err != nil {
        respondError(w, 500, "INTERNAL_ERROR", err.Error())
        return
    }
    w.Header().Set("Location", fmt.Sprintf("/api/v1/workflows/%d", resp.JobID))
    respondJSON(w, 202, resp)
}
```

**Routes:**
```go
r.Route("/api/v1/workflows", func(r chi.Router) {
    r.Post("/preprocess", s.preprocessHandler)
    r.Post("/index", s.indexHandler)
    r.Post("/eval", s.evalHandler)
    r.Get("/{id}", s.workflowStatusHandler)
})
```

All three POST endpoints return `202 Accepted` on success. The response includes `job_id`, `tag`, `state` (always `"available"` initially), and `created_at`. A `Location` header points to the status endpoint.

---

### Phase 5: Workflow Status Endpoint

**Files:** (same as Phase 4 — added to `handler_workflow.go`)

**Goal:** `GET /api/v1/workflows/:id` returns River job state for any inserted job.

```go
type JobStatusResponse struct {
    JobID       int64    `json:"job_id"`
    Kind        string   `json:"kind,omitempty"`
    State       string   `json:"state"`
    AttemptedAt string   `json:"attempted_at"`
    CompletedAt string   `json:"completed_at,omitempty"`
    Errors      []string `json:"errors,omitempty"`
}
```

State mapping from `rivertype.JobState`:
- `available` → `"available"` (queued, not yet picked up)
- `running` → `"running"`
- `retryable` → `"retrying"`
- `completed` → `"completed"`
- `cancelled` → `"cancelled"`
- `discarded` → `"failed"`

Errors are extracted from `row.Errors` (each attempt's error message concatenated).

**Edge cases:**
- Job not found (`JobGet` returns `river.ErrNotFound`) → 404
- Job not yet attempted → `attempted_at` is zero value, return `null` in JSON

---

### Phase 6: Chat Endpoint — Non-Streaming

**Files:** `internal/api/service_chat.go`, `internal/api/handler_chat.go`

**Goal:** `POST /api/v1/chat` returns a complete answer synchronously. Streaming comes in Phase 7.

**`internal/api/types.go` additions:**
```go
type ChatRequest struct {
    Tag                string  `json:"tag"`
    Query              string  `json:"query"`
    ConversationID     string  `json:"conversation_id,omitempty"`
    TopK               *int    `json:"top_k,omitempty"`
    Temperature        *float64 `json:"temperature,omitempty"`
    MaxTokens          *int    `json:"max_tokens,omitempty"`
    LLMProvider        string  `json:"llm_provider,omitempty"`
    LLMModel           string  `json:"llm_model,omitempty"`
    EmbeddingProvider  string  `json:"embedding_provider,omitempty"`
    EmbeddingModel     string  `json:"embedding_model,omitempty"`
}

type ChatResponse struct {
    Answer          string         `json:"answer"`
    SourceDocuments []SourceDoc    `json:"source_documents"`
    TokenUsage      TokenUsage     `json:"token_usage"`
    LatencyMs       int64          `json:"latency_ms"`
}

type SourceDoc struct {
    DocumentPath string  `json:"document_path"`
    Score        float32 `json:"score"`
}

type TokenUsage struct {
    Prompt     int `json:"prompt"`
    Completion int `json:"completion"`
    Total      int `json:"total"`
}
```

**`internal/api/service_chat.go`:**
```go
package api

import (
    "context"
    "strings"
    "time"

    "github.com/openai/openai-go"

    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/embedder"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/generator"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/memory"
    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/retriever"
    qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
)

type ChatService struct {
    embedder  embedder.Embedder
    retriever *retriever.Retriever
    generator generator.Generator
    memory    memory.Memory
}

func NewChatService() (*ChatService, error) {
    cfg, _ := config.Load()
    emb, _ := embedder.New(config.ProviderOpenAI, "text-embedding-3-small", 1)
    qs := qstore.NewQdrantStore(cfg.QdrantAPIKey)
    qs.Connect(context.Background(), cfg.QdrantURL)
    ret, _ := retriever.New(emb, qs, "naive-search")
    gen, _ := generator.New(config.ProviderOpenAI, "gpt-4o-mini")
    mem := memory.NewRingBuffer(10)
    return &ChatService{embedder: emb, retriever: ret, generator: gen, memory: mem}, nil
}

func (s *ChatService) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
    start := time.Now()

    results, err := s.retriever.Retrieve(ctx, req.Tag, req.Query, *req.TopK)
    if err != nil {
        return nil, fmt.Errorf("retrieve: %w", err)
    }

    var messages []openai.ChatCompletionMessageParamUnion
    messages = append(messages, openai.SystemMessage("You are a helpful assistant..."))

    if req.ConversationID != "" {
        for _, turn := range s.memory.Get(req.ConversationID) {
            messages = append(messages, openai.UserMessage(turn.User.Content))
            messages = append(messages, openai.AssistantMessage(turn.Assistant.Content))
        }
    }

    var contextParts []string
    for _, r := range results {
        contextParts = append(contextParts, fmt.Sprintf("Document: %s\n%s", r.DocumentPath, r.Content))
    }
    userPrompt := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", strings.Join(contextParts, "\n\n"), req.Query)
    messages = append(messages, openai.UserMessage(userPrompt))

    completion, err := s.generator.Generate(ctx, openai.ChatCompletionNewParams{
        Messages:  messages,
        MaxTokens: openai.Int(1024),
    })
    if err != nil {
        return nil, fmt.Errorf("generate: %w", err)
    }

    answer := completion.Choices[0].Message.Content
    if req.ConversationID != "" {
        s.memory.Add(req.ConversationID, req.Query, answer)
    }

    sources := make([]SourceDoc, len(results))
    for i, r := range results {
        sources[i] = SourceDoc{DocumentPath: r.DocumentPath, Score: r.Score}
    }

    return &ChatResponse{
        Answer:          answer,
        SourceDocuments: sources,
        TokenUsage: TokenUsage{
            Prompt:     int(completion.Usage.PromptTokens),
            Completion: int(completion.Usage.CompletionTokens),
            Total:      int(completion.Usage.TotalTokens),
        },
        LatencyMs: time.Since(start).Milliseconds(),
    }, nil
}
```

**`internal/api/handler_chat.go`:**
```go
func (s *Server) chatHandler(w http.ResponseWriter, r *http.Request) {
    var req ChatRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, 400, "INVALID_JSON", "invalid request body")
        return
    }
    if req.Query == "" || req.Tag == "" {
        respondError(w, 400, "INVALID_PARAMETER", "query and tag are required")
        return
    }
    // Apply defaults
    setDefault(&req.TopK, 5)
    setDefault(&req.Temperature, 0.3)
    setDefault(&req.MaxTokens, 1024)

    resp, err := s.chat.Chat(r.Context(), req)
    if err != nil {
        respondError(w, 500, "INTERNAL_ERROR", err.Error())
        return
    }
    respondJSON(w, 200, resp)
}
```

**Important design decision:** The chat service creates its own Qdrant connection, embedder, and generator at startup. It does NOT go through River. This is a direct synchronous call (same as the existing `cmd/query/main.go`).

Provider selection: accept `llm_provider`, `embedding_provider`, `llm_model`, `embedding_model` in the request body. If empty, fall back to `LLM_PROVIDER` / `EMBEDDING_PROVIDER` env vars.

---

### Phase 7: Chat Endpoint — SSE Streaming

**Files:** `internal/api/handler_chat.go` (extended), `internal/generator/generator.go` (new method)

**Goal:** When client sends `Accept: text/event-stream`, stream the answer token-by-token.

**1. Add streaming to `Generator` interface:**

```go
// internal/generator/generator.go
type StreamCallback func(token string) error

type Generator interface {
    Generate(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error)
    GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error)
    ModelName() string
}
```

Implementation on `openAIGenerator`:
```go
func (g *openAIGenerator) GenerateStream(ctx context.Context, params openai.ChatCompletionNewParams, cb StreamCallback) (*openai.ChatCompletion, error) {
    params.Model = openai.ChatModel(g.model)
    stream := g.client.Chat.Completions.NewStreaming(ctx, params)
    defer stream.Close()

    var full strings.Builder
    for stream.Next() {
        chunk := stream.Current()
        for _, choice := range chunk.Choices {
            if choice.Delta.Content != "" {
                full.WriteString(choice.Delta.Content)
                if err := cb(choice.Delta.Content); err != nil {
                    return nil, err
                }
            }
        }
    }
    if err := stream.Err(); err != nil {
        return nil, fmt.Errorf("stream: %w", err)
    }

    // Build a synthetic ChatCompletion to return
    return &openai.ChatCompletion{
        Choices: []openai.ChatCompletionChoice{
            {Message: openai.ChatCompletionMessage{Content: full.String()}},
        },
        Usage: openai.ChatCompletionUsage{
            PromptTokens:     chunk.Usage.PromptTokens,
            CompletionTokens: chunk.Usage.CompletionTokens,
            TotalTokens:      chunk.Usage.TotalTokens,
        },
    }, nil
}
```

**2. SSE handler:**

```go
func (s *Server) chatStreamHandler(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        respondError(w, 500, "INTERNAL_ERROR", "streaming not supported")
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    sendEvent := func(event string, data any) {
        jsonData, _ := json.Marshal(data)
        fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
        flusher.Flush()
    }

    var req ChatRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        sendEvent("error", map[string]string{"code": "INVALID_JSON", "message": "invalid request body"})
        return
    }

    start := time.Now()

    // Retrieve
    results, err := s.chat.retriever.Retrieve(r.Context(), req.Tag, req.Query, *req.TopK)
    if err != nil {
        sendEvent("error", map[string]string{"code": "RETRIEVAL_FAILED", "message": err.Error()})
        return
    }

    sources := make([]SourceDoc, len(results))
    for i, r := range results {
        sources[i] = SourceDoc{DocumentPath: r.DocumentPath, Score: r.Score}
    }
    sendEvent("retrieval", map[string]any{"results": sources})

    // Build context + messages
    // ... (same as non-streaming) ...

    // Stream
    var answer string
    _, err = s.chat.generator.GenerateStream(r.Context(), openai.ChatCompletionNewParams{
        Messages:  messages,
        MaxTokens: openai.Int(int64(*req.MaxTokens)),
    }, func(token string) error {
        sendEvent("token", map[string]string{"token": token})
        answer += token
        return nil
    })
    if err != nil {
        sendEvent("error", map[string]string{"code": "GENERATION_FAILED", "message": err.Error()})
        return
    }

    if req.ConversationID != "" {
        s.chat.memory.Add(req.ConversationID, req.Query, answer)
    }

    sendEvent("done", map[string]any{
        "source_documents": sources,
        "tokens":           TokenUsage{...}, // from the returned completion
        "latency_ms":       time.Since(start).Milliseconds(),
    })
}
```

**Route:**
```go
r.Post("/api/v1/chat", s.chatStreamHandler)  // always SSE
// Or versioned: r.Post("/api/v1/chat", s.chatHandler) for non-streaming, r.Post("/api/v1/chat/stream", s.chatStreamHandler)
```

**SSE events:**
| event | When | data |
|-------|------|------|
| `retrieval` | After Qdrant returns | `{"results": [{"document_path": "...", "score": 0.92}]}` |
| `token` | Per token from LLM | `{"token": "The"}` |
| `done` | After answer is complete | `{"source_documents": [...], "tokens": {...}, "latency_ms": 2340}` |
| `error` | On any failure | `{"code": "...", "message": "..."}` |

---

### Phase 8: Eval Runs API

**Files:** `internal/api/service_eval.go`, `internal/api/handler_eval.go`

**Goal:** `GET /api/v1/eval/runs`, `GET /api/v1/eval/runs/:id`, `GET /api/v1/eval/runs/:id/compare`

Reuses `eval.EvalStore` (existing struct in `internal/eval/store.go`) to query Postgres.

**`internal/api/service_eval.go`:**
```go
package api

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/eval"
)

type EvalService struct {
    store *eval.EvalStore
    pool  *pgxpool.Pool
}

func (s *EvalService) ListRuns(ctx context.Context, limit, offset int) ([]RunSummary, int, error) {
    var total int
    err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM eval_runs`).Scan(&total)
    if err != nil {
        return nil, 0, fmt.Errorf("count runs: %w", err)
    }

    rows, err := s.pool.Query(ctx,
        `SELECT id, tag, strategy, metrics, created_at,
                (SELECT COUNT(*) FROM eval_queries WHERE run_id = eval_runs.id) AS question_count
         FROM eval_runs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
    if err != nil {
        return nil, 0, fmt.Errorf("query runs: %w", err)
    }
    defer rows.Close()

    var runs []RunSummary
    for rows.Next() {
        var r RunSummary
        // scan id, tag, strategy JSONB, metrics JSONB, created_at, question_count
        // parse JSONB into map[string]any
        runs = append(runs, r)
    }
    return runs, total, rows.Err()
}

func (s *EvalService) GetRun(ctx context.Context, id string, limit, offset int) (*RunDetail, error) {
    // 1. Query eval_runs for run metadata
    // 2. Query eval_queries WHERE run_id = $1 ORDER BY created_at LIMIT $2 OFFSET $3
    // 3. Also query total count of queries for pagination
}
```

**`internal/api/handler_eval.go`:**
```go
func (s *Server) evalListHandler(w http.ResponseWriter, r *http.Request) {
    limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
    offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
    if limit <= 0 || limit > 100 {
        limit = 20
    }

    runs, total, err := s.eval.ListRuns(r.Context(), limit, offset)
    if err != nil {
        respondError(w, 500, "INTERNAL_ERROR", err.Error())
        return
    }
    respondJSON(w, 200, map[string]any{"runs": runs, "total": total})
}

func (s *Server) evalDetailHandler(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
    offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
    if limit <= 0 || limit > 100 {
        limit = 50
    }

    run, err := s.eval.GetRun(r.Context(), id, limit, offset)
    if err != nil {
        respondError(w, 404, "NOT_FOUND", "eval run not found")
        return
    }
    respondJSON(w, 200, run)
}
```

**Compare endpoint** — accept multiple `?compare_to=<run_id>` params, fetch each run's metrics JSONB, return side by side:

```go
func (s *Server) evalCompareHandler(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    compareIDs := r.URL.Query()["compare_to"]
    allIDs := append([]string{id}, compareIDs...)

    runs := make(map[string]RunSummary)
    for _, rid := range allIDs {
        run, err := s.eval.store.GetRunMeta(r.Context(), rid)
        if err != nil {
            respondError(w, 404, "NOT_FOUND", fmt.Sprintf("run %s not found", rid))
            return
        }
        runs[rid] = *run
    }
    respondJSON(w, 200, map[string]any{"runs": runs})
}
```

No SQL pivot needed — just fetch the `metrics` JSONB column for each run and return them all. The UI compares client-side.

**Routes:**
```go
r.Route("/api/v1/eval", func(r chi.Router) {
    r.Get("/runs", s.evalListHandler)
    r.Get("/runs/{id}", s.evalDetailHandler)
    r.Get("/runs/{id}/compare", s.evalCompareHandler)
})
```

---

### Phase 9: Artifact List Endpoint

**Files:** `internal/api/service_artifact.go`, `internal/api/handler_artifact.go`

**Goal:** `GET /api/v1/artifacts` walks the `artifacts/` directory tree and returns what's available.

```go
func (s *Server) artifactListHandler(w http.ResponseWriter, r *http.Request) {
    artifactType := r.URL.Query().Get("type")   // "preprocessing" or "indexing"
    tag := r.URL.Query().Get("tag")              // optional filter

    baseDir := "artifacts"
    entries, err := os.ReadDir(baseDir)
    if err != nil {
        respondError(w, 500, "INTERNAL_ERROR", "cannot list artifacts")
        return
    }

    var artifacts []ArtifactEntry
    for _, entry := range entries {
        if !entry.IsDir() {
            continue
        }
        // entry.Name() is "preprocessing" or "indexing"
        if artifactType != "" && entry.Name() != artifactType {
            continue
        }
        tags, _ := os.ReadDir(filepath.Join(baseDir, entry.Name()))
        for _, tagEntry := range tags {
            if !tagEntry.IsDir() {
                continue
            }
            if tag != "" && tagEntry.Name() != tag {
                continue
            }
            artifactDir := filepath.Join(baseDir, entry.Name(), tagEntry.Name())
            info := ArtifactEntry{
                Type: entry.Name(),
                Tag:  tagEntry.Name(),
            }
            // For preprocessing: count .md files in output/
            outputDir := filepath.Join(artifactDir, "output")
            if fi, err := os.Stat(outputDir); err == nil && fi.IsDir() {
                fileCount := countFiles(outputDir, ".md")
                info.FileCount = &fileCount
            }
            artifacts = append(artifacts, info)
        }
    }

    respondJSON(w, 200, map[string]any{"artifacts": artifacts})
}
```

No database needed — pure filesystem walk. Response structure:

```json
{
  "artifacts": [
    {
      "type": "preprocessing",
      "tag": "pre-handbook-v2",
      "file_count": 42,
      "created_at": "2026-06-09T12:00:00Z"
    },
    {
      "type": "indexing",
      "tag": "idx-fixed-512",
      "file_count": null
    }
  ]
}
```

For indexing artifacts, there are no files to count — the Qdrant collection IS the artifact. The endpoint could check if a Qdrant collection with the tag name exists (optional enhancement).

**Route:**
```go
r.Get("/api/v1/artifacts", s.artifactListHandler)
```

---

## Middleware Stack (reference)

Ordered outermost to innermost:

1. `RequestID` — generate/propagate `X-Request-ID`
2. `StructuredLog` — log method, path, status, duration
3. `Recovery` — catch panics, return 500
4. `Timeout(60s)` — context deadline per request
5. `CORS` — allow all origins (dev)
6. Route handlers

---

## Error Handling (reference)

| HTTP Status | Code                   | When                              |
| ----------- | ---------------------- | --------------------------------- |
| 400         | `INVALID_JSON`         | Malformed request body            |
| 400         | `INVALID_PARAMETER`    | Missing / invalid fields          |
| 404         | `NOT_FOUND`            | Job / eval run / artifact missing |
| 500         | `INTERNAL_ERROR`       | Unexpected server error           |
| 503         | `SERVICE_UNAVAILABLE`  | Postgres / Qdrant down            |

Response format:

```json
{"error": {"code": "INVALID_PARAMETER", "message": "query is required"}}
```

Success format:

```json
{"data": {"job_id": 42, "state": "available", ...}}
```

---

## River Integration

The API uses `db.NewRiverClient()` (existing function) to create a `*river.Client[pgx.Tx]`. No worker registration — the API only inserts jobs and calls `JobGet` for status.

```go
rc, err := db.NewRiverClient(ctx, 3)
defer rc.Pool.Close()

// Insert
result, err := rc.Client.Insert(ctx, &workflow.PreprocessArgs{...}, nil)

// Status
row, err := rc.Client.JobGet(ctx, jobID)
```

Workers run in the separate `workerd` process — the API never calls `river.Client.Start()`.

---

## Chat / SSE

Two integration points:

### 1. Generator streaming (new code needed)

Add `GenerateStream(ctx, params, cb)` to `Generator` interface and implement it on `openAIGenerator` using `client.Chat.Completions.NewStreaming`. The callback receives each token string. The return value is the full `ChatCompletion` (for token counting).

### 2. SSE wire format

```
event: retrieval
data: {"results":[...]}

event: token
data: {"token":"The"}

event: done
data: {"source_documents":[...],"tokens":{...},"latency_ms":2340}
```

---

## Testing Strategy

### Unit tests — service layer (`internal/api/`)

Use interfaces for River client and DB pool:

```go
type jobInserter interface {
    Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*river.InsertResult, error)
    JobGet(ctx context.Context, id int64) (*rivertype.JobRow, error)
}
```

`WorkflowService` accepts `jobInserter` — tests pass a mock.

```go
type mockJobClient struct {
    insertFn func(context.Context, river.JobArgs, *river.InsertOpts) (*river.InsertResult, error)
    jobGetFn func(context.Context, int64) (*rivertype.JobRow, error)
}
func (m *mockJobClient) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*river.InsertResult, error) {
    return m.insertFn(ctx, args, opts)
}
```

### Handler tests (`internal/api/handler_*_test.go`)

Use `httptest.NewRecorder` + `httptest.NewRequest`:

```go
func TestPreprocessHandler_MissingRepoURL(t *testing.T) {
    body := `{}`
    req := httptest.NewRequest("POST", "/api/v1/workflows/preprocess", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    // wire up with mock service that returns 400
    h := &Server{workflows: &WorkflowService{client: &mockInsertFailsClient{}}}
    http.HandlerFunc(h.preprocessHandler).ServeHTTP(rec, req)

    assert.Equal(t, 400, rec.Code)
    assert.Contains(t, rec.Body.String(), "INVALID_PARAMETER")
}
```

### Integration test (`internal/api/integration_test.go`)

One file that:
1. Requires `DATABASE_URL` and `QDRANT_URL` env vars (or skips)
2. Connects to both
3. Runs `db.Migrate()`
4. Creates a `Server` with real clients
5. Issues real HTTP requests, asserts responses

Skip in short mode: `if testing.Short() { t.Skip() }`.

### SSE test

Test the streaming handler by reading events from response body:

```go
func TestChatStream(t *testing.T) {
    // Arrange: create handler with mock generator that yields 3 tokens
    req := httptest.NewRequest("POST", "/chat", bodyReader)
    rec := httptest.NewRecorder()

    handler.ServeHTTP(rec, req)

    scanner := bufio.NewScanner(rec.Body)
    var events []string
    for scanner.Scan() {
        line := scanner.Text()
        if strings.HasPrefix(line, "event: ") {
            events = append(events, strings.TrimPrefix(line, "event: "))
        }
    }
    assert.Equal(t, []string{"retrieval", "token", "token", "token", "done"}, events)
}
```

---

## Docker Compose

Add `api` service to `docker-compose.yml`. No nginx needed for API-only phase (clients call the API directly on `:8080`).

```yaml
services:
  postgres:  # unchanged
  qdrant:    # unchanged
  workerd:   # unchanged

  api:
    build:
      context: .
      dockerfile: Dockerfile
    command: ["api"]
    ports:
      - "8080:8080"
    environment:
      DATABASE_URL: postgres://rag:rag@postgres:5432/rag?sslmode=disable
      QDRANT_URL: http://qdrant:6334
      LLM_PROVIDER: ${LLM_PROVIDER:-openai}
      OPENAI_API_KEY: ${OPENAI_API_KEY:-${LLM_API_KEY:-}}
      OPENAI_BASE_URL: ${OPENAI_BASE_URL:-${LLM_BASE_URL:-https://api.openai.com}}
      GEMINI_API_KEY: ${GEMINI_API_KEY:-}
      GEMINI_BASE_URL: ${GEMINI_BASE_URL:-https://generativelanguage.googleapis.com/v1beta/openai}
      OPENROUTER_API_KEY: ${OPENROUTER_API_KEY:-}
      OPENROUTER_BASE_URL: ${OPENROUTER_BASE_URL:-https://openrouter.ai/api/v1}
      LMSTUDIO_BASE_URL: ${LMSTUDIO_BASE_URL:-http://host.docker.internal:1234/v1}
      QDRANT_API_KEY: ${QDRANT_API_KEY:-}
      EMBEDDER_RATE_LIMIT_RPM: ${EMBEDDER_RATE_LIMIT_RPM:-500}
      GENERATOR_RATE_LIMIT_RPM: ${GENERATOR_RATE_LIMIT_RPM:-30}
      LOG_LEVEL: ${LOG_LEVEL:-info}
      API_PORT: ${API_PORT:-8080}
    volumes:
      - ./workspace:/workspace
    depends_on:
      postgres:
        condition: service_healthy
      qdrant:
        condition: service_started
```

The existing Dockerfile already builds all binaries — the `api` binary is built at `./cmd/api`. Dockerfile needs this added to the build step if not already present:

```dockerfile
RUN go build -o /build/api ./cmd/api
```

---

## Summary of New Code

| Phase | Files | Lines (est.) | Depends on |
|-------|-------|-------------|------------|
| 1 | `cmd/api/main.go`, `internal/api/server.go` | 80 | existing `config`, `chi` |
| 2 | `internal/api/middleware.go`, `internal/api/response.go` | 130 | phase 1, `uuid` |
| 3 | `internal/api/handler_health.go` | 50 | phase 2, `db.Connect`, store.HealthCheck |
| 4 | `internal/api/service_workflow.go`, `internal/api/handler_workflow.go`, `internal/api/types.go` | 200 | phase 2, `db.NewRiverClient`, `workflow.PreprocessArgs` |
| 5 | (same files as 4) | 50 | phase 4 |
| 6 | `internal/api/service_chat.go`, `internal/api/handler_chat.go` | 150 | phase 2, `embedder`, `retriever`, `generator`, `memory` |
| 7 | `internal/generator/generator.go` (extend), SSE in chat handler | 100 | phase 6 |
| 8 | `internal/api/service_eval.go`, `internal/api/handler_eval.go` | 150 | phase 2, `eval.EvalStore`, `pgx` |
| 9 | `internal/api/service_artifact.go`, `internal/api/handler_artifact.go` | 100 | phase 2 |
| — | Dockerfile + docker-compose tweaks | 10 | all phases |
