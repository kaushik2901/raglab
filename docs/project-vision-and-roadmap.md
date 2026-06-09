# Project Vision & Roadmap

A durable, observable, end-to-end RAG platform for GitLab's public handbook, operated via UI/API instead of CLI.

---

## Table of Contents

1. [Vision](#vision)
2. [Architecture Overview](#architecture-overview)
3. [What's Implemented](#whats-implemented)
4. [Remaining Work](#remaining-work)
5. [Decisions](#decisions)
6. [Open Questions](#open-questions)

---

## Vision

A single platform that lets you:

- **Trigger** a preprocessing workflow with any repo URL, any included directories, and tag it (e.g. `handbook-v2`)
- **See progress** in real time, retry failed steps, browse artifacts
- **Trigger** an indexing workflow referencing that tag — it pulls the preprocessed output, chunks, embeds, stores in its own Qdrant collection (namespaced by tag)
- **Chat** with a QA agent that uses your tag to pick the right vector collection, with configurable conversation memory
- **Evaluate** every strategy permutation at every pipeline level, capture metrics, compare results
- **Observe** everything — traces, latency, cost, token usage, error rates — per workflow, per stage, per operation

---

## Architecture Overview

```
┌────────────────────────────────────────────────────────────────┐
│                     UI (Web Dashboard)                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │Workflows │  │Artifacts │  │ Evaluate │  │ Chat (QA)     │  │
│  │  trigger │  │  browse  │  │  compare  │  │  playground   │  │
│  │  monitor │  │  manage  │  │  metrics  │  │               │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬────────┘  │
└───────┼─────────────┼─────────────┼───────────────┼────────────┘
        │             │             │               │
┌───────▼─────────────▼─────────────▼───────────────▼────────────┐
│                     REST API (HTTP)                            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │Workflow  │  │Artifact  │  │Evaluation│  │ Chat (SSE)    │  │
│  │  API     │  │  API     │  │  API     │  │  streaming    │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬────────┘  │
└───────┼─────────────┼─────────────┼───────────────┼────────────┘
        │             │             │               │
┌───────▼─────────────▼─────────────▼───────────────▼────────────┐
│              Durable Execution (River Workers)                  │
│                                                                │
│  ┌──────────────────┐  ┌──────────────────┐                    │
│  │ PreprocessWorker │  │  IndexWorker     │                    │
│  │  (clone →        │  │  (parse → chunk  │                    │
│  │   preprocess →   │  │   → embed →      │                    │
│  │   verify)        │  │   store)         │                    │
│  └──────────────────┘  └──────────────────┘                    │
│                                                                │
│  ┌──────────────────┐  ┌──────────────────┐                    │
│  │  EvalWorker      │  │  Query CLI       │                    │
│  │  (batch embed →  │  │  (sync, no River) │                    │
│  │   sequential     │  │                   │                    │
│  │   search + gen   │  │                   │                    │
│  │   + judge)       │  │                   │                    │
│  └──────────────────┘  └──────────────────┘                    │
└────────────────────────────────────────────────────────────────┘
        │             │             │               │
┌───────▼─────────────▼─────────────▼───────────────▼────────────┐
│                         Backing Services                       │
│                                                                │
│  ┌────────┐  ┌────────┐  ┌────────┐  ┌──────────────────────┐  │
│  │ Qdrant │  │Postgres│  │  S3 /  │  │ OpenTelemetry        │  │
│  │(vector)│  │(meta,  │  │ Local  │  │ (traces, metrics)    │  │
│  │        │  │ River, │  │(blob)  │  │ — planned            │  │
│  │        │  │ eval)  │  │        │  │                      │  │
│  └────────┘  └────────┘  └────────┘  └──────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

### Components

| Layer         | What                              | Purpose                                                                       |
| ------------- | --------------------------------- | ----------------------------------------------------------------------------- |
| **UI**        | Web dashboard                     | Trigger workflows, browse artifacts, chat with QA agent, compare eval results |
| **API**       | REST + SSE                        | Expose all capabilities programmatically                                      |
| **Engine**    | River workers (durable)           | At-least-once execution, retries with backoff, concurrency control            |
| **Workflows** | Preprocess / Index / Eval         | Composable pipelines as single River workers                                  |
| **Backing**   | Qdrant, Postgres, Filesystem, OTel | State, storage, vectors, observability                                       |

### Key Architecture Decisions (implemented)

- **No custom workflow tables** — River's internal job tables are the single source of truth for workflow state. No `workflows` or `workflow_steps` tables.
- **No job chaining** — each River worker (PreprocessWorker, IndexWorker, EvalWorker) is a single job that performs all its steps internally (clone → preprocess → verify in one worker; parse → chunk → embed → store in one worker). Checkpointing via `river.Job.Output()` survives crashes mid-pipeline.
- **Tag-based namespacing** — preprocessing outputs go to `artifacts/preprocessing/{tag}/`, Qdrant collections are named `{tag}`. Downstream workflows reference upstream via `input_tag`.
- **CLIs are thin** — each CLI validates args, inserts a River job, and polls until completion. The `workerd` daemon runs all workers.

---

## What's Implemented

The following is already built. See `README.md` and `AGENTS.md` for full details.

| Component | What it does |
|-----------|-------------|
| **Preprocessing** | River worker: clones repo, resolves includes/shortcodes/HTML/refs, verifies output, writes cleaned markdown |
| **Indexing** | River worker: parses `.md` files, chunks (fixed word-window), embeds via OpenAI-compatible API, stores in Qdrant |
| **Evaluation** | River worker: 4-phase pipeline (batch embed → sequential search + generate + judge), stores results in Postgres (`eval_runs` + `eval_queries` tables), computes HitRate/MRR/NDCG/Precision/Recall |
| **Query (CLI)** | Sync CLI: embeds query → retrieves from Qdrant → generates answer via LLM → in-memory ring buffer for multi-turn |
| **Embedder** | OpenAI-compatible HTTP client, batching, rate-limit retry |
| **Generator** | OpenAI-compatible chat completion, rate-limit retry |
| **Retriever** | Strategy pattern (`naive-search`), wraps embedder + Qdrant search |
| **Memory** | In-memory ring buffer per conversation ID |
| **LLM Providers** | OpenAI, Gemini, OpenRouter, LM Studio (all via OpenAI-compatible API) |
| **Postgres** | App state: eval tables + River internal tables. Migration: embedded SQL + River auto-migrate |
| **Docker Compose** | `postgres:16-alpine`, `qdrant/qdrant`, `workerd` (built from Dockerfile) |

---

## Remaining Work

### Phase 4: UI + API v1

**Goal:** Full-featured web UI and REST API. Replace CLI interaction with a dashboard.

- **API server** — new `cmd/api/` (Go, `net/http` + chi or similar):
  - `POST /api/v1/workflows/preprocess` — insert River job, return workflow ID
  - `POST /api/v1/workflows/index` — insert River job, return workflow ID
  - `POST /api/v1/workflows/eval` — insert River job, return workflow ID
  - `GET /api/v1/workflows/:id` — poll River job state
  - `GET /api/v1/artifacts` — list preprocessing/index outputs by tag
  - `POST /api/v1/chat` — SSE streaming endpoint (wraps existing retriever + generator + memory)
  - `POST /api/v1/chat/stream` — streaming variant for UI
  - `GET /api/v1/eval/runs` — list eval runs
  - `GET /api/v1/eval/runs/:id` — per-run results + metrics
  - `GET /api/v1/eval/runs/:id/compare` — compare across strategies
- **Streaming** — SSE for chat responses, optionally for workflow progress
- **Web UI** — new `web/` directory (React + shadcn/ui):
  - **Dashboard** — recent workflows, system health
  - **Workflow Detail** — status, progress, logs (polled from River job state)
  - **Chat Playground** — select tag, type questions, see retrieved chunks + answer
  - **Evaluation** — trigger runs, browse results, compare strategies side-by-side
  - **Artifact Browser** — browse by tag, view metadata
- **Eval results storage** — already in Postgres (no change needed)
- **Update docker-compose.yml** — add `api` and `web` services

**Deliverables:**
- Full REST API replacing CLI usage for common operations
- Web dashboard usable for triggering workflows, chatting, and evaluating
- SSE streaming for chat

### Phase 5: Observability

**Goal:** Production-grade observability.

- **Instrument all components** with OpenTelemetry:
  - API handlers (HTTP span per endpoint)
  - River workers (span per Work function, sub-spans for each internal step)
  - Embedder / Generator calls (span with model, token count attributes)
  - Qdrant store calls (span with collection, latency)
  - Postgres queries (database span)
- **Add to docker-compose.yml:**
  - OpenTelemetry Collector (receives OTLP from Go services)
  - Prometheus (scrapes metrics)
  - Grafana (dashboards)
  - Jaeger or Tempo (trace visualization)
- **Key dashboards:**
  - Pipeline latency (P50/P95/P99 per worker type)
  - Cost tracking (token usage per model × pricing)
  - Error rate by worker / stage / API endpoint
  - Embedding + generation rate limits hit
- **Structured logging** — `slog` with correlated `workflow_id` and `trace_id`

**Deliverables:**
- Grafana dashboards for latency, cost, error rate
- Trace-based debugging across workers and API
- Cost tracking per workflow run

### Phase 6: Production Hardening

**Goal:** Polish for real-world use.

- **Persistent chat memory** — swap in-memory ring buffer for Postgres-backed storage (use existing `Memory` interface)
- **Artifact management API** — delete artifacts (cascade to Qdrant collections and filesystem)
- **Retry/cancel API for workflows** — `POST /api/v1/workflows/:id/retry` and `/cancel`
- **Blob storage abstraction** — filesystem → S3/MinIO-compatible (abstract behind interface)
- **Auth** — basic auth or API keys (keep simple)
- **Better chunking strategies** — semantic chunking, recursive chunking (beyond fixed word-window)
- **More retrieval strategies** — hybrid search (dense + sparse), query expansion
- **`--force-recreate` flag** for index collections when embedding dimensions change

---

## Decisions

### 1. Workflow Engine
**River** (`github.com/riverqueue/river`). Lightweight Go queue backed by Postgres — zero extra infra. No custom workflow tables — River's internal tables are the single source of truth. No job chaining — each pipeline is a single worker that internally sequences its steps with checkpointing.

### 2. UI Framework
**React + shadcn/ui** — use existing components where possible, avoid building custom ones.

### 3. Authentication
**None** for now. No auth layer.

### 4. Deployment
**Docker Compose only.** No Kubernetes. The whole stack lives in `docker-compose.yml`.

### 5. Chat Memory Persistence
**Postgres** (planned). Currently in-memory ring buffer; the `Memory` interface allows swapping to Postgres-backed persistence.

### 6. Blob Storage
**Filesystem** for now. Abstract behind an interface if/when S3/MinIO is needed later.

### 7. Qdrant Collection Namespacing
**Separate collection per tag** (e.g. `handbook-v2`, `company-policies`). Payload filtering is reserved for hybrid search / metadata filtering within a collection, not for tag-based isolation.

### 8. LLM Providers
**OpenAI-compatible API only** (OpenAI, OpenRouter, LM Studio, Gemini via compatibility endpoint). No provider-specific adapters.

### 9. Monitoring Stack
**Self-hosted** via Docker Compose — OpenTelemetry Collector + Prometheus + Grafana + Jaeger, all as containers (planned).

### 10. Inter-service Communication
**HTTP/REST only.** No gRPC.

---

## Open Questions

1. **API auth model** — simple API key header? Skip auth entirely for MVP?
2. **Web UI hosting** — served by the Go API server (embed static files), or separate service?
3. **Qdrant collection management** — should the API support creating/deleting collections directly, or only through workflows?
4. **Multi-user** — any concept of users/teams, or single-tenant only?
5. **Cost tracking** — pull token pricing from an external source, or maintain a hardcoded table?
