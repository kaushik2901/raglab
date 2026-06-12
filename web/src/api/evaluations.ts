import { apiFetch } from "./client"
import type { RunSummary, RunDetail } from "./types"

interface ListRunsResponse {
  runs: RunSummary[]
  total: number
}

export async function fetchRuns(limit = 20, offset = 0) {
  const result = await apiFetch<ListRunsResponse>(
    `/api/v1/eval/runs?limit=${limit}&offset=${offset}`
  )
  return result
}

export async function fetchRunDetail(id: string, limit = 50, offset = 0) {
  return apiFetch<RunDetail>(
    `/api/v1/eval/runs/${encodeURIComponent(id)}?limit=${limit}&offset=${offset}`
  )
}

export async function compareRuns(baseId: string, targetIds: string[]) {
  const params = new URLSearchParams()
  for (const tid of targetIds) {
    params.append("compare_to", tid)
  }
  const result = await apiFetch<{ runs: Record<string, RunSummary> }>(
    `/api/v1/eval/runs/${encodeURIComponent(baseId)}/compare?${params.toString()}`
  )
  return result
}

export async function deleteRun(id: string) {
  return apiFetch<{ deleted: string }>(`/api/v1/eval/runs/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
}
