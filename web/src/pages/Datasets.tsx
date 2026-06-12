import { useState } from "react"
import { useDatasets, useUploadDataset, useDeleteDataset } from "@/hooks/useDatasets"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { ConfirmDialog } from "@/components/ConfirmDialog"
import { FileUpload } from "@/components/FileUpload"
import { DataTable, type Column } from "@/components/DataTable"
import { RiMoreLine, RiDatabase2Line } from "@remixicon/react"
import type { DatasetEntry } from "@/api/types"

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export default function Datasets() {
  const { data, isLoading } = useDatasets()
  const uploadMutation = useUploadDataset()
  const deleteMutation = useDeleteDataset()

  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-32" />
        <Skeleton className="h-64 w-full rounded-lg" />
      </div>
    )
  }

  const columns: Column<DatasetEntry>[] = [
    {
      key: "name",
      header: "Name",
      sortable: true,
      accessor: (ds) => <span className="font-medium text-sm">{ds.name}</span>,
    },
    {
      key: "size",
      header: "Size",
      sortable: true,
      numeric: true,
      accessor: (ds) => <span className="text-sm text-muted-foreground tabular-nums">{formatBytes(ds.size)}</span>,
    },
    {
      key: "questions",
      header: "Questions",
      sortable: true,
      numeric: true,
      accessor: (ds) => <span className="text-sm tabular-nums">{ds.question_count.toLocaleString()}</span>,
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      accessor: (ds) => (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <RiMoreLine className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(ds.name)}
            >
              Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ]

  const datasets = data ?? []

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Datasets</h1>
        <p className="text-sm text-muted-foreground mt-1">
          JSONL evaluation datasets for running RAG evaluations.
        </p>
      </div>

      <FileUpload
        onUpload={(file) => uploadMutation.mutate(file)}
        uploading={uploadMutation.isPending}
      />

      {datasets.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 border rounded-lg bg-muted/20">
          <RiDatabase2Line className="size-10 text-muted-foreground/40 mb-4" />
          <p className="text-sm font-medium">No datasets yet</p>
          <p className="text-xs text-muted-foreground mt-1">Upload a JSONL evaluation dataset to get started.</p>
        </div>
      ) : (
        <DataTable
          columns={columns}
          data={datasets}
          rowKey={(ds) => ds.name}
          searchPlaceholder="Filter datasets..."
          searchAccessor={(ds) => ds.name}
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
        title="Delete dataset?"
        description={`Delete dataset "${deleteTarget}"? Evaluations that used this dataset will still exist in Postgres.`}
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
