import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import * as api from "@/api/datasets"

export function useDatasets() {
  return useQuery({
    queryKey: ["datasets"],
    queryFn: api.fetchDatasets,
  })
}

export function useUploadDataset() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.uploadDataset,
    onSuccess: (data) => {
      toast.success(`Uploaded: ${data.name} (${data.question_count} questions, ${formatBytes(data.size)})`)
      queryClient.invalidateQueries({ queryKey: ["datasets"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}

export function useDeleteDataset() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.deleteDataset,
    onSuccess: (_, name) => {
      toast.success(`Deleted ${name}`)
      queryClient.invalidateQueries({ queryKey: ["datasets"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
