import { apiFetch } from "./client"
import type { DatasetEntry } from "./types"

export async function fetchDatasets(): Promise<DatasetEntry[]> {
  const result = await apiFetch<{ datasets: DatasetEntry[] }>("/api/v1/datasets")
  return result.datasets
}

export async function uploadDataset(file: File): Promise<DatasetEntry> {
  const formData = new FormData()
  formData.append("file", file)

  return apiFetch<DatasetEntry>("/api/v1/datasets", {
    method: "POST",
    body: formData,
  })
}

export async function deleteDataset(name: string) {
  return apiFetch<{ deleted: string }>(`/api/v1/datasets/${encodeURIComponent(name)}`, {
    method: "DELETE",
  })
}
