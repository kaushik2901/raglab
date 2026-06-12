import { useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useEvalRunDetail } from "@/hooks/useEvaluations"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { MetricsCards } from "@/components/MetricsCards"
import { RiArrowLeftLine } from "@remixicon/react"

export default function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const [page, setPage] = useState(1)
  const { data: run, isLoading } = useEvalRunDetail(id, page)

  if (isLoading || !run) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  const metrics = run.metrics as Record<string, number> | null

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/evaluations">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <div>
          <h2 className="text-lg font-semibold">{run.tag}</h2>
          <p className="text-xs text-muted-foreground">
            {run.question_count} questions · {run.created_at ? new Date(run.created_at).toLocaleDateString() : ""}
          </p>
        </div>
      </div>

      <MetricsCards
        metrics={[
          { label: "HitRate@5", value: metrics?.hit_rate_5?.toFixed(2) ?? "—" },
          { label: "MRR", value: metrics?.mrr?.toFixed(3) ?? "—" },
          { label: "NDCG@5", value: metrics?.ndcg_5?.toFixed(3) ?? "—" },
          { label: "Answer Score", value: metrics?.avg_answer_score?.toFixed(1) ?? "—", subtitle: "avg" },
        ]}
      />

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-12">#</TableHead>
            <TableHead>Question</TableHead>
            <TableHead>Category</TableHead>
            <TableHead>NDCG</TableHead>
            <TableHead>Score</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {run.questions.map((q: Record<string, unknown>, i: number) => (
            <TableRow key={i}>
              <TableCell className="text-muted-foreground text-xs">{(page - 1) * 50 + i + 1}</TableCell>
              <TableCell className="max-w-md truncate font-medium">
                {String(q.question ?? "—")}
              </TableCell>
              <TableCell className="text-muted-foreground text-xs">
                {String(q.category ?? "—")}
              </TableCell>
              <TableCell>
                {typeof q.ndcg_graded === "number" ? (q.ndcg_graded as number).toFixed(3) : "—"}
              </TableCell>
              <TableCell>
                {typeof q.answer_score === "number" ? (q.answer_score as number).toFixed(1) : "—"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      {run.total > 50 && (
        <div className="flex justify-between items-center">
          <Button variant="outline" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            ← Prev
          </Button>
          <span className="text-sm text-muted-foreground">
            {(page - 1) * 50 + 1}–{Math.min(page * 50, run.total)} of {run.total}
          </span>
          <Button variant="outline" disabled={page * 50 >= run.total} onClick={() => setPage((p) => p + 1)}>
            Next →
          </Button>
        </div>
      )}
    </div>
  )
}
