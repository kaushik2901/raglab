import { apiFetch } from "./client"
import type { CollectionInfo } from "./types"

export async function fetchIndexes() {
  return apiFetch<CollectionInfo[]>("/api/v1/indexes")
}

export async function fetchIndex(name: string) {
  return apiFetch<CollectionInfo>(`/api/v1/indexes/${encodeURIComponent(name)}`)
}

export async function deleteIndex(name: string) {
  return apiFetch<{ deleted: string }>(`/api/v1/indexes/${encodeURIComponent(name)}`, {
    method: "DELETE",
  })
}
