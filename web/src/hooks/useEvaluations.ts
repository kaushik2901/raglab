import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import * as api from "@/api/evaluations"

export function useEvalRuns(page = 1, limit = 20) {
  const offset = (page - 1) * limit
  return useQuery({
    queryKey: ["eval-runs", page, limit],
    queryFn: () => api.fetchRuns(limit, offset),
  })
}

export function useEvalRunDetail(id: string | undefined, page = 1, limit = 50) {
  const offset = (page - 1) * limit
  return useQuery({
    queryKey: ["eval-run", id, page, limit],
    queryFn: () => api.fetchRunDetail(id!, limit, offset),
    enabled: id != null,
  })
}

export function useCompareRuns(baseId: string | null, targetIds: string[]) {
  return useQuery({
    queryKey: ["eval-compare", baseId, ...targetIds],
    queryFn: () => api.compareRuns(baseId!, targetIds),
    enabled: baseId != null && targetIds.length > 0,
  })
}

export function useDeleteEvalRun() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.deleteRun,
    onSuccess: () => {
      toast.success(`Deleted eval run`)
      queryClient.invalidateQueries({ queryKey: ["eval-runs"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}
