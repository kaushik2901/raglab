import { apiFetch } from "./client"
import type { JobEntry, WorkflowResponse } from "./types"

export interface PreprocessPayload {
  repo_url: string
  tag: string
  base_url?: string
  include_dirs?: string[]
}

export interface IndexPayload {
  input_tag: string
  tag: string
  parser_strategy: string
  chunk_strategy: string
  chunk_config: Record<string, unknown>
  embedding_provider: string
  embedding_model: string
  batch_size: number
  index_concurrency: number
  doc_timeout: string
}

export interface EvalPayload {
  index_tag: string
  tag: string
  query_strategy: string
  dataset_path: string
  ks: number[]
  llm_provider: string
  llm_model: string
  embedding_provider: string
  embedding_model: string
  judge_provider: string
  judge_model: string
  batch_size: number
  workers: number
}

export async function createPreprocess(payload: PreprocessPayload) {
  return apiFetch<WorkflowResponse>("/api/v1/workflows/preprocess", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function createIndex(payload: IndexPayload) {
  return apiFetch<WorkflowResponse>("/api/v1/workflows/index", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function createEval(payload: EvalPayload) {
  return apiFetch<WorkflowResponse>("/api/v1/workflows/eval", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function fetchJob(id: number) {
  return apiFetch<JobEntry>(`/api/v1/workflows/${id}`)
}

export interface FetchJobsParams {
  kind?: string
  state?: string
  limit?: number
  offset?: number
}

export async function fetchJobs(params?: FetchJobsParams) {
  const search = new URLSearchParams()
  if (params?.kind) search.set("kind", params.kind)
  if (params?.state) search.set("state", params.state)
  if (params?.limit) search.set("limit", String(params.limit))
  if (params?.offset) search.set("offset", String(params.offset))
  const qs = search.toString()
  const result = await apiFetch<{ jobs: JobEntry[]; total: number }>(`/api/v1/workflows${qs ? "?" + qs : ""}`)
  return result.jobs
}
