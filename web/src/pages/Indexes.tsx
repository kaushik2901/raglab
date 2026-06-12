import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useIndexes, useDeleteIndex } from "@/hooks/useIndexes"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { JobBadge } from "@/components/JobBadge"
import { ConfirmDialog } from "@/components/ConfirmDialog"
import { DataTable, type Column } from "@/components/DataTable"
import { RiAddLine, RiMoreLine, RiSearch2Line } from "@remixicon/react"
import type { CollectionInfo } from "@/api/types"

interface PendingIndex extends CollectionInfo {
  pending: boolean
  job: { id: number; state: string }
}

export default function Indexes() {
  const { data, isLoading } = useIndexes()
  const deleteMutation = useDeleteIndex()
  const navigate = useNavigate()

  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

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
      key: "name",
      header: "Name",
      sortable: true,
      accessor: (i) => <span className="font-medium text-sm">{i.name}</span>,
    },
    {
      key: "vectors",
      header: "Vectors",
      sortable: true,
      numeric: true,
      accessor: (i) => {
        const isPending = "pending" in i && i.pending
        if (isPending) return <span className="text-sm text-muted-foreground">—</span>
        return <span className="text-sm tabular-nums">{i.vector_count > 0 ? i.vector_count.toLocaleString() : "—"}</span>
      },
    },
    {
      key: "dimensions",
      header: "Dimensions",
      sortable: true,
      numeric: true,
      accessor: (i) => {
        const isPending = "pending" in i && i.pending
        if (isPending) return <span className="text-sm text-muted-foreground">—</span>
        return <span className="text-sm tabular-nums">{i.vector_size > 0 ? i.vector_size : "—"}</span>
      },
    },
    {
      key: "distance",
      header: "Distance",
      sortable: true,
      accessor: (i) => {
        const isPending = "pending" in i && i.pending
        if (isPending) return <span className="text-sm text-muted-foreground">—</span>
        return <span className="text-sm text-muted-foreground">{i.distance || "—"}</span>
      },
    },
    {
      key: "status",
      header: "Status",
      accessor: (i) => {
        const isPending = "pending" in i && i.pending
        if (isPending) {
          const pending = i as PendingIndex
          return <JobBadge state={pending.job.state} />
        }
        return <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"><span className="flex size-2 rounded-full bg-success" />Ready</span>
      },
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      accessor: (i) => (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <RiMoreLine className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(i.name)}
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
          <h1 className="text-2xl font-bold tracking-tight">Indexes</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Vector indexes in Qdrant with their dimensions and document counts.
          </p>
        </div>
        <Button onClick={() => navigate("/indexes/new")}>
          <RiAddLine className="size-4" />
          New Index
        </Button>
      </div>

      {data.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 border rounded-lg bg-muted/20">
          <RiSearch2Line className="size-10 text-muted-foreground/40 mb-4" />
          <p className="text-sm font-medium">No indexes yet</p>
          <p className="text-xs text-muted-foreground mt-1 mb-4">Create a vector index from a preprocessed artifact.</p>
          <Button onClick={() => navigate("/indexes/new")}>
            <RiAddLine className="size-4" />
            Create Index
          </Button>
        </div>
      ) : (
        <DataTable
          columns={columns}
          data={data}
          rowKey={(i) => i.name}
          searchPlaceholder="Filter indexes..."
          searchAccessor={(i) => i.name}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
        title="Delete index?"
        description={`Delete index "${deleteTarget}"? All vectors will be removed from Qdrant. Evaluations that used this index will still exist but cannot be re-run.`}
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
