import { useState } from "react"
import { useDatasets, useUploadDataset, useDeleteDataset } from "@/hooks/useDatasets"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { EmptyState } from "@/components/EmptyState"
import { ConfirmDialog } from "@/components/ConfirmDialog"
import { FileUpload } from "@/components/FileUpload"

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
        <h2 className="text-lg font-semibold">Datasets</h2>
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Datasets</h2>
      </div>

      <FileUpload
        onUpload={(file) => uploadMutation.mutate(file)}
        uploading={uploadMutation.isPending}
      />

      {(!data || data.length === 0) ? (
        <EmptyState
          title="No datasets yet"
          description="Upload a JSONL evaluation dataset file to get started."
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Size</TableHead>
              <TableHead>Questions</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.map((ds) => (
              <TableRow key={ds.name}>
                <TableCell className="font-medium">{ds.name}</TableCell>
                <TableCell className="text-muted-foreground">{formatBytes(ds.size)}</TableCell>
                <TableCell>{ds.question_count.toLocaleString()}</TableCell>
                <TableCell>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" className="size-8">
                        ⋮
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
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
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
