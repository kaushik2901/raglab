import { useState } from "react"
import { useNavigate, Link } from "react-router-dom"
import { useEvalRuns, useDeleteEvalRun } from "@/hooks/useEvaluations"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Checkbox } from "@/components/ui/checkbox"
import { EmptyState } from "@/components/EmptyState"
import { ConfirmDialog } from "@/components/ConfirmDialog"

function metricDisplay(val: unknown): string {
  if (val == null) return "—"
  if (typeof val === "number") return val.toFixed(2)
  return String(val)
}

export default function RunList() {
  const { data, isLoading } = useEvalRuns()
  const deleteMutation = useDeleteEvalRun()
  const navigate = useNavigate()

  const runs = data?.runs ?? []

  const [baseId, setBaseId] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)

  const toggleSelect = (id: string) => {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else if (next.size < 5) next.add(id)
    setSelectedIds(next)
  }

  const handleBaseChange = (id: string) => {
    setBaseId(id)
    setSelectedIds((prev) => {
      const next = new Set(prev)
      next.add(id)
      return next
    })
  }

  const compareIds = Array.from(selectedIds).filter((id) => id !== baseId)

  if (isLoading) {
    return (
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Evaluations</h2>
        </div>
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Evaluations</h2>
        <Button asChild>
          <Link to="/evaluations/new">+ New Evaluation</Link>
        </Button>
      </div>

      {runs.length === 0 ? (
        <EmptyState
          title="No evaluations yet"
          description="Run an evaluation against an index using a dataset."
          actionLabel="Create Evaluation"
          onAction={() => navigate("/evaluations/new")}
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">Base</TableHead>
              <TableHead className="w-10">Incl</TableHead>
              <TableHead>Tag</TableHead>
              <TableHead>Dataset</TableHead>
              <TableHead>MRR</TableHead>
              <TableHead>NDCG@5</TableHead>
              <TableHead>Answer Score</TableHead>
              <TableHead>Date</TableHead>
              <TableHead className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.map((run) => {
              const metrics = run.metrics as Record<string, number> | null
              const dataset = (run.strategy as Record<string, unknown>)?.dataset_path as string ?? "—"
              return (
                <TableRow key={run.id}>
                  <TableCell>
                    <RadioGroup value={baseId ?? ""} onValueChange={handleBaseChange}>
                      <RadioGroupItem value={run.id} id={`base-${run.id}`} className="size-3.5" />
                    </RadioGroup>
                  </TableCell>
                  <TableCell>
                    <Checkbox
                      checked={selectedIds.has(run.id)}
                      onCheckedChange={() => toggleSelect(run.id)}
                      disabled={selectedIds.has(run.id) && run.id === baseId}
                    />
                  </TableCell>
                  <TableCell className="font-medium">
                    <Link to={`/evaluations/${run.id}`} className="hover:underline">
                      {run.tag}
                    </Link>
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">{dataset}</TableCell>
                  <TableCell>{metrics?.mrr != null ? metricDisplay(metrics.mrr) : "—"}</TableCell>
                  <TableCell>{metrics?.["ndcg_5"] != null ? metricDisplay(metrics["ndcg_5"]) : "—"}</TableCell>
                  <TableCell>{metrics?.avg_answer_score != null ? metricDisplay(metrics.avg_answer_score) : "—"}</TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {run.created_at ? new Date(run.created_at).toLocaleDateString() : "—"}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" className="size-8">⋮</Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => navigate(`/evaluations/${run.id}`)}>
                          View
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          className="text-destructive"
                          onClick={() => setDeleteTarget(run.id)}
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

      {baseId && compareIds.length > 0 && (
        <div className="fixed bottom-4 left-1/2 -translate-x-1/2">
          <Button
            size="lg"
            onClick={() => navigate(`/evaluations/${baseId}/compare?${compareIds.map((id) => `compare_to=${id}`).join("&")}`)}
          >
            Compare {compareIds.length} run{compareIds.length > 1 ? "s" : ""} against base
          </Button>
        </div>
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}
        title="Delete evaluation run?"
        description="All question results will be cascade-deleted from Postgres. This cannot be undone."
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
