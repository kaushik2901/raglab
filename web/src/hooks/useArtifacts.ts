import { useMemo } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import * as api from "@/api/artifacts"
import { useJobs } from "./useWorkflows"
import type { ArtifactEntry } from "@/api/types"

interface PendingArtifact extends ArtifactEntry {
  pending: boolean
  job: { id: number; state: string }
}

export function useArtifacts() {
  const diskQuery = useQuery({
    queryKey: ["artifacts", "disk"],
    queryFn: () => api.fetchArtifacts(),
  })

  const jobsQuery = useJobs("preprocess")

  const merged = useMemo(() => {
    const diskData: (ArtifactEntry | PendingArtifact)[] = diskQuery.data ?? []
    const diskTags = new Set(diskData.map((a) => a.tag))

    const pending: PendingArtifact[] = (jobsQuery.data ?? [])
      .filter((j) => !diskTags.has(j.tag))
      .map((j) => ({
        type: "preprocessing",
        tag: j.tag,
        file_count: null,
        pending: true,
        job: { id: j.id, state: j.state },
      }))

    return [...diskData, ...pending]
  }, [diskQuery.data, jobsQuery.data])

  return {
    data: merged,
    isLoading: diskQuery.isLoading || jobsQuery.isLoading,
    isError: diskQuery.isError || jobsQuery.isError,
  }
}

export function useDeleteArtifact() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ type, tag }: { type: string; tag: string }) => api.deleteArtifact(type, tag),
    onSuccess: (_, vars) => {
      toast.success(`Deleted artifact ${vars.tag}`)
      queryClient.invalidateQueries({ queryKey: ["artifacts"] })
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })
}
