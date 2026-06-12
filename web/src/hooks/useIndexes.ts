import { useMemo } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import * as api from "@/api/indexes"
import { useJobs } from "./useWorkflows"
import type { CollectionInfo } from "@/api/types"

interface PendingIndex extends CollectionInfo {
  pending: boolean
  job: { id: number; state: string }
}

export function useIndexes() {
  const qdrantQuery = useQuery({
    queryKey: ["indexes", "qdrant"],
    queryFn: api.fetchIndexes,
  })

  const jobsQuery = useJobs("index")

  const merged = useMemo(() => {
    const qdrantData: (CollectionInfo | PendingIndex)[] = qdrantQuery.data ?? []
    const qdrantNames = new Set(qdrantData.map((i) => i.name))

    const pending: PendingIndex[] = (jobsQuery.data ?? [])
      .filter((j) => !qdrantNames.has(j.tag))
      .map((j) => ({
        name: j.tag,
        vector_count: 0,
        vector_size: 0,
        distance: "",
        pending: true,
        job: { id: j.id, state: j.state },
      }))

    return [...qdrantData, ...pending]
  }, [qdrantQuery.data, jobsQuery.data])

  return {
    data: merged,
    isLoading: qdrantQuery.isLoading || jobsQuery.isLoading,
    isError: qdrantQuery.isError || jobsQuery.isError,
  }
}

export function useDeleteIndex() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: api.deleteIndex,
    onSuccess: (_, name) => {
      toast.success(`Deleted index ${name}`)
      queryClient.invalidateQueries({ queryKey: ["indexes"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}
