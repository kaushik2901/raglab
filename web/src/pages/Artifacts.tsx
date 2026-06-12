import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useArtifacts, useDeleteArtifact } from "@/hooks/useArtifacts"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { JobBadge } from "@/components/JobBadge"
import { ConfirmDialog } from "@/components/ConfirmDialog"
import { DataTable, type Column } from "@/components/DataTable"
import { RiAddLine, RiMoreLine, RiFolderOpenLine } from "@remixicon/react"
import type { ArtifactEntry } from "@/api/types"

interface PendingArtifact extends ArtifactEntry {
  pending: boolean
  job: { id: number; state: string }
}

export default function Artifacts() {
  const { data, isLoading } = useArtifacts()
  const deleteMutation = useDeleteArtifact()
  const navigate = useNavigate()

  const [deleteTarget, setDeleteTarget] = useState<{ type: string; tag: string } | null>(null)

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-32" />
        <Skeleton className="h-64 w-full rounded-lg" />
      </div>
    )
  }

  const columns: Column<(typeof data)[number]>[] = [
    {
      key: "tag",
      header: "Tag",
      sortable: true,
      accessor: (a) => <span className="font-medium text-sm">{a.tag}</span>,
    },
    {
      key: "type",
      header: "Type",
      sortable: true,
      accessor: (a) => <span className="text-xs text-muted-foreground capitalize">{a.type}</span>,
    },
    {
      key: "files",
      header: "Files",
      sortable: true,
      numeric: true,
      accessor: (a) => <span className="text-sm tabular-nums">{a.file_count != null ? a.file_count.toLocaleString() : "—"}</span>,
    },
    {
      key: "status",
      header: "Status",
      accessor: (a) => {
        const isPending = "pending" in a && a.pending
        if (isPending) {
          const pending = a as PendingArtifact
          return <JobBadge state={pending.job.state} />
        }
        return <span className="text-xs text-muted-foreground">Ready</span>
      },
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      accessor: (a) => (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <RiMoreLine className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget({ type: a.type, tag: a.tag })}
            >
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ]

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Artifacts</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Preprocessed repositories ready for indexing.
          </p>
        </div>
        <Button onClick={() => navigate("/artifacts/new")}>
          <RiAddLine className="size-4" />
          New Artifact
        </Button>
      </div>

      {data.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 border rounded-lg bg-muted/20">
          <RiFolderOpenLine className="size-10 text-muted-foreground/40 mb-4" />
          <p className="text-sm font-medium">No artifacts yet</p>
          <p className="text-xs text-muted-foreground mt-1 mb-4">Preprocess a repository to get started.</p>
          <Button onClick={() => navigate("/artifacts/new")}>
            <RiAddLine className="size-4" />
            Create Artifact
          </Button>
        </div>
      ) : (
        <DataTable
          columns={columns}
          data={data}
          rowKey={(a) => a.tag}
          searchPlaceholder="Filter artifacts..."
          searchAccessor={(a) => a.tag}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
        title="Delete artifact?"
        description={`Delete artifact ${deleteTarget?.type}/${deleteTarget?.tag}? This cannot be undone. Associated indexes will remain in Qdrant.`}
        onConfirm={() => {
          if (deleteTarget) {
            deleteMutation.mutate(deleteTarget)
            setDeleteTarget(null)
          }
        }}
      />
    </div>
  )
}
