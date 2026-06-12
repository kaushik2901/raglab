import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import * as api from "@/api/workflows"
import { toast } from "sonner"

export function useJobs(kind?: string, state = "available,running") {
  return useQuery({
    queryKey: ["workflows", { kind, state }],
    queryFn: () => api.fetchJobs({ kind, state, limit: 100 }),
    refetchInterval: (query) => {
      const hasRunning = query.state.data?.some(
        (j) => j.state !== "completed" && j.state !== "cancelled" && j.state !== "failed"
      )
      return hasRunning ? 5_000 : false
    },
  })
}

export function useJob(id: number | null) {
  return useQuery({
    queryKey: ["workflows", id],
    queryFn: () => api.fetchJob(id!),
    enabled: id != null,
    refetchInterval: (query) => {
      const terminal = ["completed", "cancelled", "failed"]
      if (query.state.data && terminal.includes(query.state.data.state)) return false
      return 2_000
    },
  })
}

export function useCreatePreprocess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.createPreprocess,
    onSuccess: (data) => {
      toast.success(`Preprocessing started — job #${data.job_id}`)
      queryClient.invalidateQueries({ queryKey: ["workflows"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}

export function useCreateIndex() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.createIndex,
    onSuccess: (data) => {
      toast.success(`Indexing started — job #${data.job_id}`)
      queryClient.invalidateQueries({ queryKey: ["workflows"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}

export function useCreateEval() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.createEval,
    onSuccess: (data) => {
      toast.success(`Evaluation started — job #${data.job_id}`)
      queryClient.invalidateQueries({ queryKey: ["workflows"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}
