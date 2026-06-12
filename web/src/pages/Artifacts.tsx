import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useArtifacts, useDeleteArtifact } from "@/hooks/useArtifacts"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { EmptyState } from "@/components/EmptyState"
import { ConfirmDialog } from "@/components/ConfirmDialog"
import { JobBadge } from "@/components/JobBadge"

export default function Artifacts() {
  const { data, isLoading } = useArtifacts()
  const deleteMutation = useDeleteArtifact()
  const navigate = useNavigate()

  const [deleteTarget, setDeleteTarget] = useState<{ type: string; tag: string } | null>(null)

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Artifacts</h2>
        </div>
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Artifacts</h2>
        <Button onClick={() => navigate("/artifacts/new")}>+ New Artifact</Button>
      </div>

      {data.length === 0 ? (
        <EmptyState
          title="No artifacts yet"
          description="Preprocess a repository to get started."
          actionLabel="Create Artifact"
          onAction={() => navigate("/artifacts/new")}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Tag</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Files</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.map((a) => {
              const isPending = "pending" in a && a.pending
              const pending = a as typeof a & { job: { id: number; state: string } }
              return (
                <TableRow key={a.tag}>
                  <TableCell className="font-medium">{a.tag}</TableCell>
                  <TableCell className="text-muted-foreground">{a.type}</TableCell>
                  <TableCell>{a.file_count != null ? a.file_count.toLocaleString() : "—"}</TableCell>
                  <TableCell>
                    {isPending ? (
                      <JobBadge state={pending.job.state} />
                    ) : (
                      <span className="text-xs text-muted-foreground">completed</span>
                    )}
                  </TableCell>
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
                          onClick={() => setDeleteTarget({ type: a.type, tag: a.tag })}
                        >
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
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
