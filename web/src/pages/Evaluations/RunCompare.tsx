import { Link, useParams, useSearchParams } from "react-router-dom"
import { useCompareRuns } from "@/hooks/useEvaluations"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Separator } from "@/components/ui/separator"
import {
  RadarChart,
  PolarGrid,
  PolarAngleAxis,
  PolarRadiusAxis,
  Radar,
  Legend,
  ResponsiveContainer,
} from "recharts"
import { RiArrowLeftLine, RiLineChartLine } from "@remixicon/react"
import { cn } from "@/lib/utils"
import type { RunSummary } from "@/api/types"

const CHART_COLORS = ["var(--chart-1)", "var(--chart-2)", "var(--chart-3)", "var(--chart-4)", "var(--chart-5)"]

function getMetric(metrics: Record<string, unknown> | null, key: string): number {
  if (!metrics) return 0
  const parts = key.split(".")
  let val: any = metrics
  for (const part of parts) {
    val = val?.[part]
    if (val == null) return 0
  }
  return typeof val === "number" ? val : 0
}

function formatDelta(base: number, comp: number, _higherIsBetter: boolean): string {
  const diff = comp - base
  if (Math.abs(diff) < 0.001) return "0"
  const sign = diff >= 0 ? "+" : ""
  return `${sign}${diff.toFixed(2)}`
}

function deltaColor(base: number, comp: number, higherIsBetter: boolean): string {
  const diff = comp - base
  if (Math.abs(diff) < 0.001) return "text-muted-foreground"
  const improved = higherIsBetter ? diff > 0 : diff < 0
  return improved ? "text-success" : "text-destructive"
}

const RADAR_METRICS = [
  { key: "HitRate.5", label: "HitRate@5" },
  { key: "MRR", label: "MRR" },
  { key: "NDCG.5", label: "NDCG@5" },
  { key: "AvgAnswerScore", label: "Answer Score" },
]

function buildRadarData(runMap: Record<string, RunSummary>, baseId: string, targetIds: string[]) {
  const baseRun = runMap[baseId]
  if (!baseRun) return []

  return RADAR_METRICS.map((m) => {
    const entry: Record<string, any> = { metric: m.label }
    const baseVal = getMetric(baseRun.metrics, m.key)
    entry["Base"] = baseVal
    for (const tid of targetIds) {
      const target = runMap[tid]
      if (target) entry[target.tag.length > 14 ? target.tag.slice(0, 14) + "..." : target.tag] = getMetric(target.metrics, m.key)
    }
    return entry
  })
}

export default function RunCompare() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const compareTo = searchParams.getAll("compare_to")

  const { data, isLoading } = useCompareRuns(id ?? null, compareTo)

  if (isLoading || !data) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <Skeleton className="h-80 rounded-lg" />
          <Skeleton className="h-80 rounded-lg" />
        </div>
      </div>
    )
  }

  const runMap = data.runs
  const base = runMap[id!]
  const targets = compareTo.map((tid) => runMap[tid]).filter(Boolean)

  if (!base) {
    return (
      <div className="flex flex-col items-center justify-center py-24 text-center">
        <RiLineChartLine className="size-10 text-muted-foreground/40 mb-4" />
        <p className="text-muted-foreground">Base run not found.</p>
        <Button variant="outline" className="mt-4" asChild>
          <Link to="/evaluations">Back to evaluations</Link>
        </Button>
      </div>
    )
  }

  const radarData = buildRadarData(runMap, id!, compareTo)

  const comparisonRows = [
    { label: "HitRate@5", key: "HitRate.5", higherIsBetter: true, fmt: (v: number) => v.toFixed(2) },
    { label: "MRR", key: "MRR", higherIsBetter: true, fmt: (v: number) => v.toFixed(3) },
    { label: "NDCG@5", key: "NDCG.5", higherIsBetter: true, fmt: (v: number) => v.toFixed(3) },
    { label: "Avg Answer Score", key: "AvgAnswerScore", higherIsBetter: true, fmt: (v: number) => v.toFixed(1) },
    { label: "Avg Latency", key: "AvgLatencyMs", higherIsBetter: false, fmt: (v: number) => `${Math.round(v)}ms` },
    { label: "Avg Prompt Tokens", key: "AvgPromptTokens", higherIsBetter: false, fmt: (v: number) => Math.round(v).toString() },
    { label: "Avg Comp Tokens", key: "AvgCompletionTokens", higherIsBetter: false, fmt: (v: number) => Math.round(v).toString() },
  ]

  return (
    <div className="space-y-8">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/evaluations">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Comparison</h1>
          <p className="text-sm text-muted-foreground">
            {targets.length + 1} evaluation runs · base: <span className="font-medium text-foreground">{base.tag}</span>
          </p>
        </div>
      </div>

      <Separator />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Multi-Metric Radar</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer width="100%" height={350}>
              <RadarChart data={radarData}>
                <PolarGrid className="stroke-border" />
                <PolarAngleAxis dataKey="metric" tick={{ fontSize: 11 }} className="fill-muted-foreground" />
                <PolarRadiusAxis angle={30} domain={[0, 1]} tick={{ fontSize: 10 }} className="fill-muted-foreground" tickCount={5} />
                <Radar
                  name={base.tag}
                  dataKey="Base"
                  stroke={CHART_COLORS[0]}
                  fill={CHART_COLORS[0]}
                  fillOpacity={0.15}
                  strokeWidth={2}
                />
                {targets.map((t, i) => (
                  <Radar
                    key={t.id}
                    name={t.tag.length > 14 ? t.tag.slice(0, 14) + "..." : t.tag}
                    dataKey={t.tag.length > 14 ? t.tag.slice(0, 14) + "..." : t.tag}
                    stroke={CHART_COLORS[(i + 1) % CHART_COLORS.length]}
                    fill={CHART_COLORS[(i + 1) % CHART_COLORS.length]}
                    fillOpacity={0.1}
                    strokeWidth={2}
                  />
                ))}
                <Legend wrapperStyle={{ fontSize: "12px" }} />
              </RadarChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Metric Delta vs Base</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {comparisonRows.map((row) => {
                const baseVal = getMetric(base.metrics, row.key)
                const maxDelta = targets.reduce((max, t) => {
                  const tv = getMetric(t.metrics, row.key)
                  return Math.max(max, Math.abs(tv - baseVal))
                }, 0)

                return (
                  <div key={row.key}>
                    <div className="flex items-center justify-between mb-1.5">
                      <span className="text-xs font-medium">{row.label}</span>
                      <span className="text-xs text-muted-foreground tabular-nums">
                        Base: {row.fmt(baseVal)}
                      </span>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-xs font-semibold tabular-nums w-16 shrink-0">{row.fmt(baseVal)}</span>
                      {targets.map((t) => {
                        const tv = getMetric(t.metrics, row.key)
                        const diff = tv - baseVal
                        const pct = maxDelta > 0 ? (Math.abs(diff) / maxDelta) * 60 : 0
                        const isImproved = row.higherIsBetter ? diff > 0 : diff < 0

                        return (
                          <div key={t.id} className="flex-1 flex items-center gap-2">
                            <div className="flex-1 h-5 bg-muted rounded-full overflow-hidden relative">
                              {diff !== 0 && (
                                <div
                                  className={cn(
                                    "h-full rounded-full absolute transition-all",
                                    isImproved ? "bg-success/60" : "bg-destructive/60"
                                  )}
                                  style={{
                                    width: `${pct}%`,
                                    [diff > 0 ? "left" : "right"]: "50%",
                                    [diff > 0 ? "right" : "left"]: "auto",
                                  }}
                                />
                              )}
                              <div className="absolute inset-0 flex items-center justify-center">
                                <span className={cn("text-[10px] font-medium tabular-nums", deltaColor(baseVal, tv, row.higherIsBetter))}>
                                  {formatDelta(baseVal, tv, row.higherIsBetter)}
                                </span>
                              </div>
                            </div>
                            <span className="text-xs tabular-nums w-16 text-right font-mono text-muted-foreground">{row.fmt(tv)}</span>
                          </div>
                        )
                      })}
                    </div>
                  </div>
                )
              })}
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium">Detail Table</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b bg-muted/40">
                  <th className="text-left py-3 px-4 font-medium text-xs uppercase tracking-wider text-muted-foreground">Metric</th>
                  <th className="text-right py-3 px-4 font-medium text-xs uppercase tracking-wider text-muted-foreground bg-muted/20">
                    {base.tag}
                  </th>
                  {targets.map((t) => (
                    <th key={t.id} className="text-right py-3 px-4 font-medium text-xs uppercase tracking-wider text-muted-foreground">
                      {t.tag}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {comparisonRows.map((row) => {
                  const baseVal = getMetric(base.metrics, row.key)
                  return (
                    <tr key={row.key} className="border-b hover:bg-muted/20 transition-colors">
                      <td className="py-2.5 px-4 font-medium">{row.label}</td>
                      <td className="py-2.5 px-4 text-right tabular-nums font-semibold bg-muted/10">
                        {row.fmt(baseVal)}
                      </td>
                      {targets.map((t) => {
                        const tv = getMetric(t.metrics, row.key)
                        return (
                          <td key={t.id} className="py-2.5 px-4 text-right tabular-nums">
                            {row.fmt(tv)}
                            <span className={cn("ml-2 text-xs", deltaColor(baseVal, tv, row.higherIsBetter))}>
                              {formatDelta(baseVal, tv, row.higherIsBetter)}
                              {" "}
                              {tv > baseVal ? (row.higherIsBetter ? "↑" : "↓") : tv < baseVal ? (row.higherIsBetter ? "↓" : "↑") : ""}
                            </span>
                          </td>
                        )
                      })}
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
