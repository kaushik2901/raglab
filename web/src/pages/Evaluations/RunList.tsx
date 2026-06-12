import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useEvalRuns, useDeleteEvalRun } from "@/hooks/useEvaluations"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import { Checkbox } from "@/components/ui/checkbox"
import { ConfirmDialog } from "@/components/ConfirmDialog"
import { DataTable, type Column } from "@/components/DataTable"
import { RiAddLine, RiMoreLine, RiLineChartLine } from "@remixicon/react"
import { cn } from "@/lib/utils"
import type { RunSummary } from "@/api/types"

function MetricBadge({ value, digits = 2, colorClass = "bg-muted" }: { value: unknown; digits?: number; colorClass?: string }) {
  if (value == null) return <span className="text-sm text-muted-foreground">—</span>
  const display = typeof value === "number" ? value.toFixed(digits) : String(value)
  return (
    <span className={cn("inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium tabular-nums", colorClass)}>
      {display}
    </span>
  )
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
        <Skeleton className="h-8 w-32" />
        <Skeleton className="h-64 w-full rounded-lg" />
      </div>
    )
  }

  const columns: Column<RunSummary>[] = [
    {
      key: "base",
      header: "Base",
      headerClassName: "w-14",
      className: "w-14",
      accessor: (run) => (
        <RadioGroup value={baseId ?? ""} onValueChange={handleBaseChange}>
          <RadioGroupItem value={run.id} id={`base-${run.id}`} className="size-3.5" />
        </RadioGroup>
      ),
    },
    {
      key: "incl",
      header: "Incl",
      headerClassName: "w-14",
      className: "w-14",
      accessor: (run) => (
        <Checkbox
          checked={selectedIds.has(run.id)}
          onCheckedChange={() => toggleSelect(run.id)}
          disabled={selectedIds.has(run.id) && run.id === baseId}
        />
      ),
    },
    {
      key: "tag",
      header: "Tag",
      sortable: true,
      accessor: (run) => (
        <button
          className="text-sm font-medium hover:text-primary hover:underline text-left"
          onClick={() => navigate(`/evaluations/${run.id}`)}
        >
          {run.tag}
        </button>
      ),
    },
    {
      key: "hr",
      header: "HR@5",
      sortable: true,
      numeric: true,
      accessor: (run) => {
        const m = run.metrics as Record<string, any> | null
        return <MetricBadge value={m?.HitRate?.["5"]} colorClass="bg-blue-50 text-blue-700 dark:bg-blue-950 dark:text-blue-300" />
      },
    },
    {
      key: "mrr",
      header: "MRR",
      sortable: true,
      numeric: true,
      accessor: (run) => {
        const m = run.metrics as Record<string, any> | null
        return <MetricBadge value={m?.MRR} digits={3} colorClass="bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300" />
      },
    },
    {
      key: "ndcg",
      header: "NDCG@5",
      sortable: true,
      numeric: true,
      accessor: (run) => {
        const m = run.metrics as Record<string, any> | null
        return <MetricBadge value={m?.NDCG?.["5"]} digits={3} colorClass="bg-violet-50 text-violet-700 dark:bg-violet-950 dark:text-violet-300" />
      },
    },
    {
      key: "score",
      header: "Score",
      sortable: true,
      numeric: true,
      accessor: (run) => {
        const m = run.metrics as Record<string, any> | null
        return <MetricBadge value={m?.AvgAnswerScore} digits={1} colorClass="bg-amber-50 text-amber-700 dark:bg-amber-950 dark:text-amber-300" />
      },
    },
    {
      key: "date",
      header: "Date",
      sortable: true,
      accessor: (run) => (
        <span className="text-xs text-muted-foreground">
          {run.created_at ? new Date(run.created_at).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" }) : "—"}
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      className: "w-10",
      accessor: (run) => (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <RiMoreLine className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => navigate(`/evaluations/${run.id}`)}>
              View Details
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive"
              onClick={() => setDeleteTarget(run.id)}
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
          <h1 className="text-2xl font-bold tracking-tight">Evaluations</h1>
          <p className="text-sm text-muted-foreground mt-1">
            RAG evaluation runs with retrieval, generation, and answer quality metrics.
          </p>
        </div>
        <Button onClick={() => navigate("/evaluations/new")}>
          <RiAddLine className="size-4" />
          New Evaluation
        </Button>
      </div>

      {runs.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-24 border rounded-lg bg-muted/20">
          <RiLineChartLine className="size-10 text-muted-foreground/40 mb-4" />
          <p className="text-sm font-medium">No evaluations yet</p>
          <p className="text-xs text-muted-foreground mt-1 mb-4">Run an evaluation against an index using a dataset.</p>
          <Button onClick={() => navigate("/evaluations/new")}>
            <RiAddLine className="size-4" />
            Create Evaluation
          </Button>
        </div>
      ) : (
        <DataTable
          columns={columns}
          data={runs}
          rowKey={(r) => r.id}
          searchPlaceholder="Filter by tag..."
          searchAccessor={(r) => r.tag}
        />
      )}

      {baseId && compareIds.length > 0 && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-50">
          <Button
            size="lg"
            className="shadow-lg"
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
