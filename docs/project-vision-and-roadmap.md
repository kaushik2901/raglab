# Project Vision & Roadmap

A durable, observable, end-to-end RAG platform for GitLab's public handbook, operated via UI/API instead of CLI. Built bottom-up from modular components, then assembled into managed workflows.

---

## Table of Contents

1. [Vision](#vision)
2. [Architecture Overview](#architecture-overview)
3. [Workflow Engine](#workflow-engine)
4. [Workflows](#workflows)
5. [UI / API Layer](#ui--api-layer)
6. [Evaluation System](#evaluation-system)
7. [Observability](#observability)
8. [Phase Plan](#phase-plan)
9. [Open Questions](#open-questions)

---

## Vision

A single platform that lets you:

- **Trigger** a preprocessing workflow with any repo URL, any included directories, and tag it (e.g. `handbook-v2`)
- **See progress** in real time, retry failed steps, browse artifacts
- **Trigger** an indexing workflow referencing that tag — it pulls the preprocessed output, chunks, embeds, stores in its own Qdrant collection (namespaced by tag)
- **Chat** with a QA agent that uses your tag to pick the right vector collection, with in-memory conversation history
- **Evaluate** every strategy permutation at every pipeline level, capture metrics, compare results
- **Observe** everything — traces, latency, cost, token usage, error rates — per workflow, per stage, per operation

---

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────┐
│                     UI (Web Dashboard)                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐   │
│  │Workflows │  │Artifacts │  │ Evaluate │  │ Chat (QA)     │   │
│  │  trigger │  │  browse  │  │  compare │  │  playground   │   │
│  │  monitor │  │  manage  │  │  metrics │  │               │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬────────┘   │
└───────┼─────────────┼─────────────┼───────────────┼────────────┘
        │             │             │               │
┌───────▼─────────────▼─────────────▼───────────────▼────────────┐
│                     REST API (HTTP / gRPC)                     │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐   │
│  │Workflow  │  │Artifact  │  │Evaluation│  │ Chat (SSE)    │   │
│  │  API     │  │  API     │  │  API     │  │  streaming    │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬────────┘   │
└───────┼─────────────┼─────────────┼───────────────┼────────────┘
        │             │             │               │
┌───────▼─────────────▼─────────────▼───────────────▼────────────┐
│              Durable Execution Engine                          │
│                                                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │ Preprocess  │  │  Index      │  │   Query / Chat          │ │
│  │  Workflow   │  │  Workflow   │  │   Workflow              │ │
│  │             │  │             │  │                         │ │
│  │ clone       │  │ parse       │  │ embed query             │ │
│  │ preprocess  │  │ chunk       │  │ retrieve (Qdrant)       │ │
│  │ verify      │  │ embed       │  │ generate (LLM)          │ │
│  │             │  │ store       │  │ return + memory update  │ │
│  └──────┬──────┘  └──────┬──────┘  └────────────┬────────────┘ │
└─────────┼────────────────┼──────────────────────┼──────────────┘
          │                │                      │
┌─────────▼────────────────▼──────────────────────▼──────────────┐
│                         Backing Services                       │
│                                                                │
│  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐  ┌──────────┐  │
│  │ Qdrant │  │Postgres│  │  S3 /  │  │ Redis  │  │ OpenTele-│  │
│  │(vector)│  │(meta,  │  │ Local  │  │ (queue,│  │ metry    │  │
│  │        │  │workflow│  │(blob)  │  │ cache) │  │ (traces, │  │
│  │        │  │ state) │  │        │  │        │  │ metrics) │  │
│  └────────┘  └────────┘  └────────┘  └────────┘  └──────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### Components

| Layer         | What                              | Purpose                                                                       |
| ------------- | --------------------------------- | ----------------------------------------------------------------------------- |
| **UI**        | Web dashboard                     | Trigger workflows, browse artifacts, chat with QA agent, compare eval results |
| **API**       | REST + SSE                        | Expose all capabilities programmatically                                      |
| **Engine**    | Durable execution                 | Run, resume, retry long-running workflows; persist state                      |
| **Workflows** | Preprocess / Index / Query        | Composable pipelines of stages                                                |
| **Backing**   | Qdrant, Postgres, S3, Redis, OTel | State, storage, queues, observability                                         |

### Durable Execution Engine (River-based)

The engine is built on **River** (`github.com/riverqueue/river`) — a lightweight Go job queue backed by Postgres.

#### Architecture

```
┌──────────────────────────────────────────────┐
│           API / CLI / UI Trigger             │
└──────────────────┬───────────────────────────┘
                   │ INSERT workflow row + first job
                   ▼
┌──────────────────────────────────────────────┐
│          Postgres (single source of truth)   │
│                                              │
│  workflows: id, type, tag, status, params    │
│  workflow_steps: id, workflow_id, name,      │
│                  status, error, attempts     │
│  river_job (managed by River)                │
└──────────────┬───────────────────────────────┘
               │ River worker picks up next job
               ▼
┌──────────────────────────────────────────────┐
│           River Worker Pool                  │
│                                              │
│  ┌──────────┐  ┌───────────┐  ┌────────────┐ │
│  │ CloneWkr │→ │Preprocess │→ │ VerifyWkr  │ │
│  │          │  │ Wkr       │  │            │ │
│  └──────────┘  └───────────┘  └────────────┘ │
│                                              │
│  On success: enqueue next step               │
│  On failure: River retries                   │
│  On terminal failure: mark workflow          │
│    failed, log error                         │
└──────────────────────────────────────────────┘
```

#### How it works

1. **Workflow trigger** inserts a row in the `workflows` table and enqueues a River job for step 1.
2. **Worker executes** the step logic. On success, it:
   - Updates `workflow_steps` with status + result
   - Enqueues the next step as a new River job (carrying the `workflow_id` in its args)
3. **River guarantees** at-least-once delivery, retries with backoff, and concurrency control via PG advisory locks.
4. **On crash**: River picks up incomplete jobs when the worker restarts. Workflow state is fully reconstructed from Postgres.
5. **Live status**: The API queries `workflows` + `workflow_steps` tables directly (no River-specific query needed).

#### DAG coordination

Since River is a flat queue (not a DAG engine), a thin coordination layer handles step sequencing:

```go
// WorkerWorkflowStep enqueues the next step on success
func (w *PreprocessWorker) Work(ctx context.Context, job *river.Job[PreprocessArgs]) error {
    result, err := runPreprocessStep(ctx, job.Args)
    if err != nil {
        return err // River handles retry
    }
    // Enqueue next step
    _, err = river.Client.Insert(ctx, &VerifyArgs{WorkflowID: job.Args.WorkflowID})
    return err
}
```

For linear pipelines (preprocess, index, query), this is sufficient. Future branching/parallel steps can be added with a lightweight DAG resolver that enqueues multiple children and waits for all to complete.

#### Guarantees

- **Durable progress** — state in Postgres survives crashes
- **Step-level retry** — River's native retry with configurable backoff
- **Live status** — UI polls `/api/v1/workflows/:id` for step statuses from Postgres
- **Tag-based linking** — workflows accept and produce tags carried in job args

#### Tag Model

Each pipeline has its **own identity tag** for its output artifacts.
Downstream pipelines reference upstream outputs via `input_tag`, not by sharing the same tag.

```
# Pre-processing — produces cleaned markdown, tagged "pre-v2"
Preprocess( tag: "pre-v2", repo: "...", includeDirs: [...] )
    → clones to: artifacts/preprocessing/pre-v2/repo/
    → writes output to: artifacts/preprocessing/pre-v2/output/
    → output: { tag: "pre-v2", repo_path: "artifacts/preprocessing/pre-v2/repo/", output_path: "artifacts/preprocessing/pre-v2/output/" }

# Indexing — consumes preprocessed output, produces a Qdrant collection, tagged "idx-v2"
Index( tag: "idx-v2", input_tag: "pre-v2", chunk_size: 512, ... )
    → reads from artifacts/preprocessing/pre-v2/output/
    → stores vectors in Qdrant collection "idx-v2"
    → output: { tag: "idx-v2", collection: "idx-v2" }

# Query — uses an indexed collection
Query( tag: "idx-v2", question: "..." )
    → queries Qdrant collection "idx-v2"
```

This CI/CD-like model means:

- **One preprocessing run, many indexing strategies**: `Index(tag="idx-v2-fixed-512", input_tag="pre-v2")` and `Index(tag="idx-v2-recursive-256", input_tag="pre-v2")` reuse the same cleaned markdown.
- **Each pipeline owns its artifact namespace**: preprocessing outputs go under `artifacts/preprocessing/{tag}/`, Qdrant collections are named `{tag}`. No collision.
- **Full traceability**: given a query tag, you can look up which index params were used, and from that which preprocessed artifacts.

Tags can be:

- **User-specified** — `pre-handbook-v2`, `idx-handbook-v2-fixed-512`, `eval-recursive-256-3small`
- **Auto-generated** — `pre-20260603-143022` if not provided.

#### Artifact Management

Each workflow produces artifacts:

- **Preprocessing**: cleaned markdown files (blob storage), verification report
- **Indexing**: Qdrant collection (vector store), index metadata (model used, chunk params, embedding dims)
- **Querying**: conversation logs, retrieval traces

The API supports:

- **List** artifacts by tag, workflow type, date range
- **Read** artifact metadata and (for blobs) content
- **Delete** artifacts — cascades to clean up associated Qdrant collections and blob storage

---

## Workflows

### 1. Preprocessing Workflow

| Step                | Description                                           |
| ------------------- | ----------------------------------------------------- |
| `validate_input`    | Validate repo URL, auth, disk space                   |
| `clone`             | Clone / pull repo                                     |
| `preprocess`        | Transform markdown (includes, shortcodes, HTML, refs) |
| `verify`            | Quality checks on output                              |
| `publish_artifacts` | Tag and publish cleaned output to blob store          |

**Input parameters:**

- `repo_url` (string, required)
- `include_dirs` (string[], optional, default: all)
- `tag` (string, optional, default: auto-generated)

Paths are derived from the tag:

| Artifact | Path |
|----------|------|
| Cloned repo | `artifacts/preprocessing/{tag}/repo/` |
| Preprocessed output | `artifacts/preprocessing/{tag}/output/` |
| Verification report | `artifacts/preprocessing/{tag}/output/_verification_report.json` |

**Output artifacts:**

- Cleaned markdown files (filesystem)
- Verification report (filesystem)
- Workflow metadata (Postgres)

### 2. Indexing Workflow

| Step                | Description                                           |
| ------------------- | ----------------------------------------------------- |
| `resolve_artifacts` | Look up preprocessed artifacts by input tag           |
| `parse`             | Walk preprocessed output, read docs                   |
| `chunk`             | Split docs into chunks (fixed strategy)               |
| `embed`             | Generate embeddings via API                           |
| `store`             | Upsert into Qdrant collection                         |
| `publish_metadata`  | Register collection name, params in artifact registry |

**Input parameters:**

- `tag` (string, required) — this indexing run's own identity (e.g. `idx-v2-fixed-512`)
- `input_tag` (string, required) — references a preprocessing run's tag (e.g. `pre-v2`)
- `chunk_strategy`, `chunk_size`, `chunk_overlap`
- `embedding_model`, `batch_size`
- `llm_base_url`, `llm_api_key`

**Output artifacts:**

- Qdrant collection reference
- Index metadata (model, dims, chunk params, doc count)

### 3. Query / Chat Workflow

| Step                 | Description                                        |
| -------------------- | -------------------------------------------------- |
| `resolve_collection` | Look up latest Qdrant collection for the given tag |
| `embed_query`        | Embed the user's question                          |
| `retrieve`           | Semantic search against the collection             |
| `build_context`      | Assemble retrieved chunks into a prompt            |
| `generate`           | Call LLM with context + conversation history       |
| `update_memory`      | Append to in-memory conversation buffer            |

**Input parameters:**

- `tag` (string, required) — an indexing run's tag, identifies which Qdrant collection to query (e.g. `idx-v2-fixed-512`)
- `query` (string, required)
- `conversation_id` (string, optional) — for multi-turn memory
- `top_k`, `temperature`, `max_tokens` (optional)

**Output:**

- Streamed answer (SSE)
- Retrieved chunks with scores
- Token usage, latency

**Memory:** Simple in-memory ring buffer per `conversation_id` (last N turns). Future: persistent memory in Postgres.

---

## UI / API Layer

### API Endpoints (proposed)

```
Workflows
  POST   /api/v1/workflows/preprocess        → trigger, returns workflow_id
  POST   /api/v1/workflows/index             → trigger, returns workflow_id
  POST   /api/v1/workflows/query             → trigger chat, returns SSE stream
  GET    /api/v1/workflows/:id               → status, progress, step details
  GET    /api/v1/workflows                   → list, filter by tag/type/status
  POST   /api/v1/workflows/:id/retry         → retry failed step
  POST   /api/v1/workflows/:id/cancel        → cancel running workflow

Artifacts
  GET    /api/v1/artifacts                   → list, filter by tag/type
  GET    /api/v1/artifacts/:id               → metadata
  GET    /api/v1/artifacts/:id/download      → download blob (if applicable)
  DELETE /api/v1/artifacts/:id               → cascade delete (blob + Qdrant)

Evaluation
  POST   /api/v1/eval/run                    → trigger eval suite
  GET    /api/v1/eval/runs                   → list eval runs
  GET    /api/v1/eval/runs/:id               → results, metrics
  GET    /api/v1/eval/runs/:id/compare       → compare across strategies

Observability
  GET    /api/v1/metrics                     → aggregated pipeline metrics
  GET    /api/v1/traces                      → trace query by workflow_id
```

### UI Pages (proposed)

| Page                | Description                                                |
| ------------------- | ---------------------------------------------------------- |
| **Dashboard**       | Recent workflows, system health, quick stats               |
| **Workflow Detail** | DAG visualization, step status, logs, retry button         |
| **Artifacts**       | Browse by tag, delete, download                            |
| **Chat Playground** | Select tag, type questions, see retrieved chunks + answer  |
| **Evaluation**      | Trigger eval runs, compare results in tables/charts        |
| **Metrics**         | Cost, latency, error rate dashboards (grafana or embedded) |

### Technology Decisions

| Concern                | Decision                                                              |
| ---------------------- | --------------------------------------------------------------------- |
| **UI framework**       | React + shadcn/ui                                                     |
| **API framework (Go)** | `net/http` with a router (chi / gorilla)                              |
| **Streaming**          | SSE for chat responses and workflow progress                          |
| **Auth**               | None (for now)                                                        |
| **Database**           | Postgres — workflow state, metadata, eval results, chat memory, etc.  |
| **Deployment**         | Docker Compose only                                                   |
| **Inter-service comms**| HTTP/REST only (no gRPC)                                              |

---

## Evaluation System

### Goal

Systematically compare strategies at every pipeline level to make data-driven decisions.

### Dimensions to Evaluate

| Level          | What to vary                                                  | Metrics                                            |
| -------------- | ------------------------------------------------------------- | -------------------------------------------------- |
| **Chunking**   | size, overlap, strategy (fixed → semantic, recursive)         | chunk count, avg tokens/chunk, content integrity   |
| **Embedding**  | model (text-embedding-3-small, 3-large, ada-002, open-source) | cost, latency, retrieval quality                   |
| **Retrieval**  | top-K, similarity metric, hybrid search                       | precision@K, recall@K, MRR, NDCG                   |
| **Generation** | model, prompt template, temperature, context window           | answer relevance, faithfulness, hallucination rate |

### Process

1. **Define a ground-truth dataset** — a set of (question, expected answer, relevant document IDs) pairs drawn from the handbook.
2. **Run indexed eval** — for each strategy combination, run the full index pipeline, then run queries against it.
3. **Capture metrics** — at each level, store structured results in Postgres.
4. **Compare** — the eval UI shows side-by-side comparisons with charts.

### Metrics Storage Schema (proposed)

```sql
CREATE TABLE eval_runs (
    id            UUID PRIMARY KEY,
    strategy_name TEXT NOT NULL,      -- e.g. "fixed-512-64-text-embedding-3-small"
    parameters    JSONB NOT NULL,     -- full param snapshot
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE eval_queries (
    id           UUID PRIMARY KEY,
    run_id       UUID REFERENCES eval_runs(id),
    question     TEXT NOT NULL,
    expected     TEXT NOT NULL,
    retrieved    JSONB NOT NULL,      -- [{chunk_id, score, doc_path, content}]
    answer       TEXT NOT NULL,
    metrics      JSONB NOT NULL       -- {precision, recall, faithfulness, latency, cost}
);
```

---

## Observability

### Pillars

| Pillar      | What                                                                                     | Tooling                                           |
| ----------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------- |
| **Traces**  | End-to-end request tracing across all components (API → workflow → stage → Qdrant / LLM) | OpenTelemetry (OTel SDK) + Jaeger / Grafana Tempo |
| **Metrics** | Counters, histograms, gauges per operation                                               | OTel metrics → Prometheus → Grafana               |
| **Logs**    | Structured, correlated by workflow_id and trace_id                                       | `slog` with OTel integration                      |
| **Cost**    | Per-workflow token usage × model pricing                                                 | Custom tracking; logged as structured events      |
| **Latency** | P50/P95/P99 per stage and per API endpoint                                               | OTel histogram metrics                            |

### Key Metrics

```
# Per workflow
workflow_duration_seconds{workflow_type, status, tag}
workflow_steps_total{workflow_type, tag}
workflow_steps_failed_total{workflow_type, tag}

# Per stage
stage_duration_seconds{stage_name, workflow_type, status}
stage_retry_count{stage_name, workflow_type}

# Embedding
embed_tokens_total{model}
embed_latency_seconds{model}
embed_cost_total{model}
embed_rate_limits_total{model}

# Retrieval
retrieval_latency_seconds{collection}
retrieval_chunks_count{collection}

# Generation
llm_prompt_tokens_total{model}
llm_completion_tokens_total{model}
llm_latency_seconds{model}
llm_cost_total{model}
```

### Architecture

All Go services use the **OpenTelemetry SDK** to:

1. Propagate trace context across service boundaries (HTTP headers / gRPC metadata)
2. Record spans for each operation (HTTP handler, workflow step, DB call, LLM call)
3. Export metrics via OTLP exporter to an OTel Collector, then to Prometheus

---

## Phase Plan

### Phase 1: River + Pipeline Workers

**Goal:** Add durability and tag-based namespacing to long-running pipelines. The existing CLIs become thin wrappers that insert River jobs and tail logs.

- **Add dependencies**: `go get github.com/riverqueue/river`, `github.com/jackc/pgx/v5`
- **Docker Compose**: add Postgres + River to `docker-compose.yml`
- **Define workflow state model** in Postgres:
  - `workflows` table (id, type, tag, status, input_params, created_at, updated_at)
  - `workflow_steps` table (id, workflow_id, step_name, status, attempts, error, started_at, completed_at)
- **Define River job args** for each workflow step:
  - `CloneArgs`, `PreprocessArgs`, `VerifyArgs`, `ParseArgs`, `ChunkArgs`, `EmbedArgs`, `StoreArgs`
  - Each carries `WorkflowID` and `Tag` (this workflow's own output identity)
  - Indexing steps also carry `InputTag` — the tag of the preprocessed artifacts to consume
- **Implement River workers** (`internal/workflow/`):
  - Each worker runs the step → updates `workflow_steps` → enqueues the next step on success
  - Steps within a pipeline share the same `Tag`: `Clone(tag="pre-v2")` → `Preprocess(tag="pre-v2")` → `Verify(tag="pre-v2")`. The tag is the pipeline's run identity.
  - Indexing steps carry both their own tag and the upstream reference: `Parse(tag="idx-v2", input_tag="pre-v2")` → `Store(tag="idx-v2", input_tag="pre-v2")`. Collection name derives from their own tag: `"idx-v2"`.
- **CLIs become thin**: `cmd/preprocess/main.go` validates args, inserts a River job, and polls until completion (or exits immediately for async usage). `cmd/index/main.go` accepts both `--tag` and `--input-tag`.
- **Artifact structure**:

  | Content | Path |
  |---------|------|
  | Cloned repo | `artifacts/preprocessing/{tag}/repo/` |
  | Preprocessed output | `artifacts/preprocessing/{tag}/output/` |
  | Qdrant collection | `{tag}` (in Qdrant, not filesystem) |

  Paths are always derived from the workflow type and tag — never hardcoded.

**Files to create:**

- `internal/workflow/workflow.go` — workflow DB helpers
- `internal/workflow/preprocess_worker.go` — clone + preprocess + verify workers
- `internal/workflow/index_worker.go` — parse + chunk + embed + store workers
- `docker-compose.yml` — add Postgres service
- Schema migrations for `workflows`, `workflow_steps` tables

**Deliverables:**

- `bin\preprocess.exe --tag pre-v2` clones to `artifacts/preprocessing/pre-v2/repo/`, writes output to `artifacts/preprocessing/pre-v2/output/`
- `bin\index.exe --tag idx-v2 --input-tag pre-v2` reads from `artifacts/preprocessing/pre-v2/output/`, creates Qdrant collection `idx-v2`
- Crash safety: restart the process and incomplete jobs resume (River + Postgres)
- Preprocessed artifacts and vector collections are namespaced by tag

### Phase 2: Query System

**Goal:** The core RAG loop — retrieve and generate. Kept as direct Go functions (no River needed for single query latency).

- **Retriever** — new `internal/retriever/`:
  - Embed query via existing `internal/embedder`
  - Semantic search against a tagged Qdrant collection
  - Return top-K chunks with scores
- **Generator** — new `internal/generator/`:
  - OpenAI-compatible chat completion
  - Build prompt from retrieved chunks + conversation history
  - Return answer + token usage
- **Memory** — new `internal/memory/`:
  - In-memory ring buffer per `conversation_id` (last N turns)
  - Interface to swap for Postgres-backed persistence later

**Files to create:**

- `internal/retriever/retriever.go`, `internal/retriever/retriever_test.go`
- `internal/generator/generator.go`, `internal/generator/generator_test.go`
- `internal/memory/memory.go`, `internal/memory/memory_test.go`

**Deliverables:**

- `retriever.Retrieve(tag, query, topK)` returns relevant chunks
- `generator.Generate(prompt)` returns answer
- All callable directly from Go code (used by eval in Phase 3)

### Phase 3: Evaluation

**Goal:** Measure retrieval and generation quality across strategy permutations. Runs on top of River (for indexing jobs) and direct function calls (for querying).

- **Ground-truth dataset** — JSON file of `{question, expected_answer, relevant_doc_ids}` pairs derived from the handbook
- **Prerequisite**: a preprocessing run with a known tag (e.g. `eval-pre-v1`). Eval references this via `input_tag`.
- **Eval harness** — new `cmd/eval/` CLI, accepts `--input-tag` to reuse preprocessed artifacts:
  - For each strategy permutation (vary `top_K`, `chunk_size`, `overlap`, `embedding_model`, `temperature`):
    1. Trigger an **indexing workflow** (via River) with:
       - `tag = eval-idx-{strategy-hash}` (unique per permutation)
       - `input_tag = <eval-pre-v1>` (reuses the same preprocessed outputs)
    2. Wait for indexing to complete (poll `workflows` table)
    3. For each question in the ground-truth set:
       - Retrieve chunks from the tagged Qdrant collection
       - Generate an answer
       - Compare against ground truth → compute precision@K, recall@K, MRR, faithfulness, latency, cost
    4. Write strategy results to a JSON results file
  - Summary table printed at the end: which strategy won on which metric
- **No UI yet** — results are JSON files, readable by humans and parseable by the future UI

**Files to create:**

- `cmd/eval/main.go`, `cmd/eval/main_test.go`
- `testdata/eval/questions.json` — ground-truth seed

**Deliverables:**

- `go run ./cmd/eval` runs N strategy combinations, prints comparison table
- Each eval run is fully tagged and traceable back to the exact pipeline params

### Phase 4: UI + API v1

**Goal:** Full-featured web UI and REST API for all capabilities.

- Build out the **API** (`cmd/api/`):
  - Workflow trigger + status endpoints
  - Chat endpoint (SSE streaming)
  - Artifact management (list, download, delete)
  - Eval trigger + results endpoints
- Build the **UI** (`web/`):
  - Dashboard, workflow detail, artifact browser, chat playground, eval comparison
- Eval results stored in Postgres instead of JSON files

**Deliverables:**

- Full API surface
- Usable web dashboard

### Phase 5: Observability

**Goal:** Production-grade observability.

- Instrument all components with OpenTelemetry
- Add OTel Collector + Prometheus + Grafana + Jaeger to docker-compose
- Cost tracking and latency dashboards

**Deliverables:**

- Grafana dashboards for latency, cost, error rate
- Trace-based debugging

---

## Decisions

### 1. Workflow Engine
**River** (`github.com/riverqueue/river`). Lightweight Go queue backed by Postgres — zero extra infra. A thin DAG layer on top handles step sequencing.

### 2. UI Framework
**React + shadcn/ui** — use existing components where possible, avoid building custom ones.

### 3. Authentication
**None** for now. No auth layer.

### 4. Deployment
**Docker Compose only.** No Kubernetes. The whole stack lives in `docker-compose.yml`.

### 5. Chat Memory Persistence
**Postgres.** Same database as everything else — no Redis needed.

### 6. Blob Storage
**Filesystem** for now. Abstract behind an interface if/when S3/MinIO is needed later.

### 7. Qdrant Collection Namespacing
**Separate collection per tag** (e.g. `handbook-v2`, `company-policies`). Payload filtering is reserved for hybrid search / metadata filtering within a collection, not for tag-based isolation.

### 8. LLM Providers
**OpenAI-compatible API only** (OpenAI, OpenRouter, LM Studio, Ollama). No provider-specific adapters.

### 9. Monitoring Stack
**Self-hosted** via Docker Compose — OpenTelemetry Collector + Prometheus + Grafana + Jaeger, all as containers.

### 10. Inter-service Communication
**HTTP/REST only.** No gRPC for internal communication.

---

## Appendix: Current State (Baseline)

### What exists today

| Component              | Status         | Details                                                          |
| ---------------------- | -------------- | ---------------------------------------------------------------- |
| Preprocessing pipeline | ✅ Done        | 3 stages (clone, preprocess, verify), CLI, journal-based resume  |
| Indexing pipeline      | ✅ Done        | 4 stages (parse, chunk, embed, store), CLI, journal-based resume |
| Embedder               | ✅ Done        | OpenAI-compatible HTTP client, batching, rate-limit retry        |
| Store                  | ✅ Done        | Qdrant gRPC backend, Collection management                       |
| Chunker                | ✅ Done        | Fixed-size word-window strategy                                  |
| Parser                 | ✅ Done        | Walks directory, reads .md files                                 |
| Evaluation             | 📄 Planned     | docs/evaluation-system-plan.md                                   |
| UI/API                 | ❌ Not started | —                                                                |
| Workflow engine        | ❌ Not started | —                                                                |
| Observability          | ❌ Not started | —                                                                |
| Chat agent             | ❌ Not started | —                                                                |

### Next immediate step

**Phase 1** — Add River + Postgres to the stack, wrap preprocessing and indexing as durable tagged workflows. This unblocks tag-based namespacing for eval (Phase 3) and gives crash safety to the long-running pipelines.
