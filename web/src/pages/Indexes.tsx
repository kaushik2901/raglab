import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useIndexes, useDeleteIndex } from "@/hooks/useIndexes"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { EmptyState } from "@/components/EmptyState"
import { ConfirmDialog } from "@/components/ConfirmDialog"
import { JobBadge } from "@/components/JobBadge"

export default function Indexes() {
  const { data, isLoading } = useIndexes()
  const deleteMutation = useDeleteIndex()
  const navigate = useNavigate()

  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Indexes</h2>
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
        <h2 className="text-lg font-semibold">Indexes</h2>
        <Button onClick={() => navigate("/indexes/new")}>+ New Index</Button>
      </div>

      {data.length === 0 ? (
        <EmptyState
          title="No indexes yet"
          description="Create a vector index from a preprocessed artifact."
          actionLabel="Create Index"
          onAction={() => navigate("/indexes/new")}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Vectors</TableHead>
              <TableHead>Dimensions</TableHead>
              <TableHead>Distance</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {data.map((idx) => {
              const isPending = "pending" in idx && idx.pending
              return (
                <TableRow key={idx.name}>
                  <TableCell className="font-medium">{idx.name}</TableCell>
                  <TableCell>
                    {isPending ? "—" : idx.vector_count > 0 ? idx.vector_count.toLocaleString() : "—"}
                  </TableCell>
                  <TableCell>{isPending ? "—" : idx.vector_size > 0 ? idx.vector_size : "—"}</TableCell>
                  <TableCell className="text-muted-foreground">{isPending ? "—" : idx.distance || "—"}</TableCell>
                  <TableCell>
                    {isPending ? <JobBadge state={idx.job.state} /> : null}
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
                          onClick={() => setDeleteTarget(idx.name)}
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
