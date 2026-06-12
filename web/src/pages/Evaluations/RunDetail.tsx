import { useState, useMemo } from "react"
import { Link, useParams } from "react-router-dom"
import { useEvalRunDetail } from "@/hooks/useEvaluations"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
  ComposedChart,
} from "recharts"
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

type QuestionRow = Record<string, unknown>

function buildScoreDist(questions: QuestionRow[]) {
  const bins = Array.from({ length: 10 }, (_, i) => ({ bin: `${i}-${i + 1}`, min: i, max: i + 1, count: 0 }))
  for (const q of questions) {
    const s = typeof q.answer_score === "number" ? q.answer_score : -1
    if (s < 0 || s > 10) continue
    const idx = Math.min(Math.floor(s), 9)
    if (idx >= 0 && idx < 10) bins[idx].count++
  }
  return bins.filter((b) => b.count > 0 || bins.some((bb) => bb.count > 0))
}

function buildLatencyDist(questions: QuestionRow[]) {
  const buckets = [
    { label: "<1s", max: 1000 },
    { label: "1-3s", max: 3000 },
    { label: "3-5s", max: 5000 },
    { label: "5-10s", max: 10000 },
    { label: ">10s", max: Infinity },
  ]
  return buckets.map((b) => ({
    label: b.label,
    count: questions.filter((q) => {
      const l = typeof q.latency_ms === "number" ? q.latency_ms : -1
      if (l < 0) return false
      const prev = buckets[buckets.indexOf(b) - 1]
      return l >= (prev?.max ?? 0) && l < b.max
    }).length,
  }))
}

function buildCategoryBreakdown(questions: QuestionRow[]) {
  const map = new Map<string, { count: number; avgScore: number; totalRank: number }>()
  for (const q of questions) {
    const cat = String(q.category ?? "Uncategorized")
    const entry = map.get(cat) ?? { count: 0, avgScore: 0, totalRank: 0 }
    entry.count++
    entry.avgScore += (typeof q.answer_score === "number" ? q.answer_score : 0)
    entry.totalRank += (typeof q.rank_first === "number" ? q.rank_first : 0)
    map.set(cat, entry)
  }
  return Array.from(map.entries())
    .map(([category, e]) => ({
      category: category.length > 20 ? category.slice(0, 20) + "..." : category,
      count: e.count,
      avgScore: e.avgScore / e.count,
      avgRank: e.totalRank / e.count,
    }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 8)
}

const METRIC_COLORS = {
  hr: "var(--chart-1)",
  mrr: "var(--chart-2)",
  ndcg: "var(--chart-3)",
  score: "var(--chart-4)",
}

const DIST_COLORS = ["var(--chart-1)", "var(--chart-2)", "var(--chart-3)", "var(--chart-5)", "var(--chart-4)"]

export default function RunDetail() {
  const { id } = useParams<{ id: string }>()
  const [page, setPage] = useState(1)
  const [expanded, setExpanded] = useState<number | null>(null)
  const { data: run, isLoading } = useEvalRunDetail(id, page, 50)

  const charts = useMemo(() => {
    if (!run?.questions) return null
    const qs = run.questions as QuestionRow[]
    return {
      scoreDist: buildScoreDist(qs),
      latencyDist: buildLatencyDist(qs),
      categoryBreakdown: buildCategoryBreakdown(qs),
    }
  }, [run?.questions])

  if (isLoading || !run) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 rounded-lg" />
          ))}
        </div>
        <Skeleton className="h-64 w-full rounded-lg" />
      </div>
    )
  }

  const metrics = run.metrics as Record<string, any> | null
  const questions = run.questions as QuestionRow[]

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/evaluations">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold tracking-tight">{run.tag}</h1>
          <p className="text-sm text-muted-foreground">
            {run.question_count} questions · {run.created_at ? new Date(run.created_at).toLocaleDateString("en-US", { year: "numeric", month: "long", day: "numeric" }) : ""}
          </p>
        </div>
      </div>

      <Separator />

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {[
          { label: "HitRate@5", value: metrics?.HitRate?.["5"], format: (v: number) => v.toFixed(2), color: METRIC_COLORS.hr },
          { label: "MRR", value: metrics?.MRR, format: (v: number) => v.toFixed(3), color: METRIC_COLORS.mrr },
          { label: "NDCG@5", value: metrics?.NDCG?.["5"], format: (v: number) => v.toFixed(3), color: METRIC_COLORS.ndcg },
          { label: "Answer Score", value: metrics?.AvgAnswerScore, format: (v: number) => v.toFixed(1), color: METRIC_COLORS.score, subtitle: "/ 10" },
        ].map((m) => (
          <Card key={m.label} className="overflow-hidden">
            <CardContent className="p-5">
              <div className="flex items-center gap-2 mb-3">
                <div className="flex size-2 rounded-full shrink-0" style={{ backgroundColor: m.color }} />
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{m.label}</p>
              </div>
              <p className="text-3xl font-bold tracking-tight tabular-nums">
                {typeof m.value === "number" ? m.format(m.value) : "—"}
              </p>
              {m.subtitle && <p className="text-xs text-muted-foreground mt-0.5">{m.subtitle}</p>}
            </CardContent>
          </Card>
        ))}
      </div>

      {charts && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">Answer Score Distribution</CardTitle>
            </CardHeader>
            <CardContent>
              {charts.scoreDist.length === 0 ? (
                <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">No data</div>
              ) : (
                <ResponsiveContainer width="100%" height={200}>
                  <BarChart data={charts.scoreDist} margin={{ top: 5, right: 10, left: -15, bottom: 0 }}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
                    <XAxis dataKey="bin" tick={{ fontSize: 10 }} tickLine={false} axisLine={false} className="fill-muted-foreground" />
                    <YAxis allowDecimals={false} tick={{ fontSize: 10 }} tickLine={false} axisLine={false} className="fill-muted-foreground" />
                    <Tooltip
                      contentStyle={{ backgroundColor: "var(--popover)", border: "1px solid var(--border)", borderRadius: "0.5rem", fontSize: "0.8rem" }}
                    />
                    <Bar dataKey="count" radius={[4, 4, 0, 0]}>
                      {charts.scoreDist.map((_, i) => (
                        <Cell key={i} fill={DIST_COLORS[i % DIST_COLORS.length]} fillOpacity={0.85} />
                      ))}
                    </Bar>
                  </BarChart>
                </ResponsiveContainer>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">Latency Distribution</CardTitle>
            </CardHeader>
            <CardContent>
              <ResponsiveContainer width="100%" height={200}>
                <BarChart data={charts.latencyDist} margin={{ top: 5, right: 10, left: -15, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
                  <XAxis dataKey="label" tick={{ fontSize: 10 }} tickLine={false} axisLine={false} className="fill-muted-foreground" />
                  <YAxis allowDecimals={false} tick={{ fontSize: 10 }} tickLine={false} axisLine={false} className="fill-muted-foreground" />
                  <Tooltip
                    contentStyle={{ backgroundColor: "var(--popover)", border: "1px solid var(--border)", borderRadius: "0.5rem", fontSize: "0.8rem" }}
                  />
                  <Bar dataKey="count" radius={[4, 4, 0, 0]} fill="var(--chart-5)" fillOpacity={0.85} />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">Per-Category Avg Score</CardTitle>
            </CardHeader>
            <CardContent>
              {charts.categoryBreakdown.length === 0 ? (
                <div className="flex items-center justify-center py-12 text-sm text-muted-foreground">No data</div>
              ) : (
                <ResponsiveContainer width="100%" height={200}>
                  <ComposedChart layout="vertical" data={charts.categoryBreakdown} margin={{ top: 5, right: 10, left: -5, bottom: 0 }}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
                    <XAxis type="number" domain={[0, 10]} tick={{ fontSize: 10 }} tickLine={false} className="fill-muted-foreground" />
                    <YAxis type="category" dataKey="category" tick={{ fontSize: 10 }} tickLine={false} axisLine={false} width={90} className="fill-muted-foreground" />
                    <Tooltip
                      contentStyle={{ backgroundColor: "var(--popover)", border: "1px solid var(--border)", borderRadius: "0.5rem", fontSize: "0.8rem" }}
                    />
                    <Bar dataKey="avgScore" name="Avg Score" radius={[0, 4, 4, 0]} fill="var(--chart-2)" fillOpacity={0.85} barSize={16} />
                  </ComposedChart>
                </ResponsiveContainer>
              )}
            </CardContent>
          </Card>
        </div>
      )}

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium">Question Results</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40">
                <TableHead className="w-8" />
                <TableHead className="w-12">#</TableHead>
                <TableHead className="w-20">QID</TableHead>
                <TableHead>Question</TableHead>
                <TableHead className="w-24">Category</TableHead>
                <TableHead className="w-20">Difficulty</TableHead>
                <TableHead className="w-14 text-right">Rank</TableHead>
                <TableHead className="w-16 text-right">NDCG</TableHead>
                <TableHead className="w-16 text-right">Score</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {questions.map((q, i) => {
                const isOpen = expanded === i
                const score = typeof q.answer_score === "number" ? q.answer_score : 0
                const scoreColor = score >= 7 ? "var(--success)" : score >= 4 ? "var(--warning)" : "var(--destructive)"

                return (
                  <>
                    <TableRow
                      key={`row-${i}`}
                      className="cursor-pointer hover:bg-muted/30 transition-colors"
                      onClick={() => setExpanded(isOpen ? null : i)}
                    >
                      <TableCell className="text-muted-foreground">
                        {isOpen ? <RiArrowDownSLine className="size-4" /> : <RiArrowRightSLine className="size-4" />}
                      </TableCell>
                      <TableCell className="text-muted-foreground text-xs tabular-nums">{(page - 1) * 50 + i + 1}</TableCell>
                      <TableCell className="text-muted-foreground text-xs font-mono">{cell(q.question_id)}</TableCell>
                      <TableCell className="max-w-md truncate font-medium text-sm">{cell(q.question)}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-xs font-normal">{cell(q.category)}</Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary" className="text-xs capitalize font-normal">{cell(q.difficulty)}</Badge>
                      </TableCell>
                      <TableCell className="text-right text-sm tabular-nums">{cell(q.rank_first)}</TableCell>
                      <TableCell className="text-right font-mono text-sm tabular-nums">{num(q.ndcg_graded, 3)}</TableCell>
                      <TableCell className="text-right">
                        <span className="text-sm font-semibold tabular-nums" style={{ color: scoreColor }}>
                          {num(q.answer_score, 1)}
                        </span>
                      </TableCell>
                    </TableRow>
                    {isOpen && (
                      <TableRow key={`detail-${i}`}>
                        <TableCell colSpan={9} className="bg-muted/20 p-5">
                          <div className="grid grid-cols-1 gap-4 text-sm">
                            <div>
                              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Expected Answer</p>
                              <div className="prose prose-sm max-w-none text-foreground leading-relaxed whitespace-pre-wrap">{cell(q.expected_answer)}</div>
                            </div>
                            <Separator />
                            <div>
                              <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Generated Answer</p>
                              <div className="prose prose-sm max-w-none text-foreground leading-relaxed whitespace-pre-wrap">{cell(q.generated_answer)}</div>
                            </div>
                            <Separator />
                            <div className="flex flex-wrap gap-x-6 gap-y-2 text-xs text-muted-foreground">
                              <span>Prompt tokens: <strong className="text-foreground tabular-nums">{cell(q.prompt_tokens)}</strong></span>
                              <span>Completion tokens: <strong className="text-foreground tabular-nums">{cell(q.completion_tokens)}</strong></span>
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
        </CardContent>
      </Card>

      {run.total > 50 && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-muted-foreground tabular-nums">
            {(page - 1) * 50 + 1}–{Math.min(page * 50, run.total)} of {run.total}
          </span>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
              Previous
            </Button>
            <span className="text-xs text-muted-foreground tabular-nums">Page {page} / {Math.ceil(run.total / 50)}</span>
            <Button variant="outline" size="sm" disabled={page * 50 >= run.total} onClick={() => setPage((p) => p + 1)}>
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
