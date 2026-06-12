import { useState } from "react"
import { Link, useParams } from "react-router-dom"
import { useEvalRunDetail } from "@/hooks/useEvaluations"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { Badge } from "@/components/ui/badge"
import { MetricsCards } from "@/components/MetricsCards"
import { RiArrowLeftLine, RiArrowDownSLine, RiArrowRightSLine } from "@remixicon/react"

function cell(v: unknown): string {
  if (v == null) return "—"
  return String(v)
}

function num(v: unknown, digits = 2): string {
  if (v == null) return "—"
  if (typeof v === "number") return v.toFixed(digits)
  return String(v)
}

export default function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const [page, setPage] = useState(1)
  const [expanded, setExpanded] = useState<number | null>(null)
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

  const metrics = run.metrics as Record<string, any> | null

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
          { label: "HitRate@5", value: metrics?.HitRate?.["5"] != null ? (metrics.HitRate["5"] as number).toFixed(2) : "—" },
          { label: "MRR", value: metrics?.MRR != null ? (metrics.MRR as number).toFixed(3) : "—" },
          { label: "NDCG@5", value: metrics?.NDCG?.["5"] != null ? (metrics.NDCG["5"] as number).toFixed(3) : "—" },
          { label: "Answer Score", value: metrics?.AvgAnswerScore != null ? (metrics.AvgAnswerScore as number).toFixed(1) : "—", subtitle: "avg" },
        ]}
      />

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-8" />
            <TableHead className="w-12">#</TableHead>
            <TableHead className="w-24">QID</TableHead>
            <TableHead>Question</TableHead>
            <TableHead className="w-24">Category</TableHead>
            <TableHead className="w-20">Diff.</TableHead>
            <TableHead className="w-16">Rank</TableHead>
            <TableHead className="w-20">NDCG</TableHead>
            <TableHead className="w-20">Score</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {run.questions.map((q: Record<string, unknown>, i: number) => {
            const isOpen = expanded === i
            return (
              <>
                <TableRow
                  key={`row-${i}`}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => setExpanded(isOpen ? null : i)}
                >
                  <TableCell className="text-muted-foreground">
                    {isOpen ? <RiArrowDownSLine className="size-4" /> : <RiArrowRightSLine className="size-4" />}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">{(page - 1) * 50 + i + 1}</TableCell>
                  <TableCell className="text-muted-foreground text-xs font-mono">{cell(q.question_id)}</TableCell>
                  <TableCell className="max-w-md truncate font-medium">{cell(q.question)}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className="text-xs">{cell(q.category)}</Badge>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary" className="text-xs capitalize">{cell(q.difficulty)}</Badge>
                  </TableCell>
                  <TableCell className="text-center">{cell(q.rank_first)}</TableCell>
                  <TableCell className="font-mono">{num(q.ndcg_graded, 3)}</TableCell>
                  <TableCell className="font-mono">{num(q.answer_score, 1)}</TableCell>
                </TableRow>
                {isOpen && (
                  <TableRow key={`detail-${i}`}>
                    <TableCell colSpan={9} className="bg-muted/20 p-4">
                      <div className="grid grid-cols-1 gap-3 text-sm">
                        <div>
                          <p className="text-xs font-medium text-muted-foreground mb-1">Expected Answer</p>
                          <p className="leading-relaxed whitespace-pre-wrap">{cell(q.expected_answer)}</p>
                        </div>
                        <div>
                          <p className="text-xs font-medium text-muted-foreground mb-1">Generated Answer</p>
                          <p className="leading-relaxed whitespace-pre-wrap">{cell(q.generated_answer)}</p>
                        </div>
                        <div className="flex gap-6 text-xs text-muted-foreground border-t pt-3">
                          <span>Prompt tokens: <strong className="text-foreground">{cell(q.prompt_tokens)}</strong></span>
                          <span>Completion tokens: <strong className="text-foreground">{cell(q.completion_tokens)}</strong></span>
                          <span>Latency: <strong className="text-foreground">{typeof q.latency_ms === "number" ? `${(q.latency_ms as number)}ms` : "—"}</strong></span>
                        </div>
                      </div>
                    </TableCell>
                  </TableRow>
                )}
              </>
            )
          })}
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
