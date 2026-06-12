import { apiFetch } from "./client"
import type { ArtifactEntry } from "./types"

export async function fetchArtifacts(params?: { type?: string; tag?: string }) {
  const search = new URLSearchParams()
  if (params?.type) search.set("type", params.type)
  if (params?.tag) search.set("tag", params.tag)
  const qs = search.toString()
  return apiFetch<ArtifactEntry[]>(`/artifacts${qs ? "?" + qs : ""}`)
}

export async function deleteArtifact(type: string, tag: string) {
  return apiFetch<{ deleted: string }>(`/api/v1/artifacts/${encodeURIComponent(type)}/${encodeURIComponent(tag)}`, {
    method: "DELETE",
  })
}
