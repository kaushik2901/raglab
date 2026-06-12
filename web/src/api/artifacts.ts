import { apiFetch } from "./client"
import type { ArtifactEntry } from "./types"

export async function fetchArtifacts(params?: { type?: string; tag?: string }) {
  const search = new URLSearchParams()
  if (params?.type) search.set("type", params.type)
  if (params?.tag) search.set("tag", params.tag)
  const qs = search.toString()
  const result = await apiFetch<{ artifacts: ArtifactEntry[] }>(`/artifacts${qs ? "?" + qs : ""}`)
  return result.artifacts
}

export async function deleteArtifact(type: string, tag: string) {
  return apiFetch<{ deleted: string }>(`/artifacts/${encodeURIComponent(type)}/${encodeURIComponent(tag)}`, {
    method: "DELETE",
  })
}
