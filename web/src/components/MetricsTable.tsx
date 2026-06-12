import { useMemo } from "react"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { cn } from "@/lib/utils"
import type { RunSummary } from "@/api/types"

interface MetricsTableProps {
  base: RunSummary
  targets: RunSummary[]
}

interface MetricRow {
  label: string
  key: string
  higherIsBetter: boolean
  format: (v: number) => string
}

const METRICS: MetricRow[] = [
  { label: "HitRate@1", key: "HitRate.1", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "HitRate@3", key: "HitRate.3", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "HitRate@5", key: "HitRate.5", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "MRR", key: "MRR", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "NDCG@5", key: "NDCG.5", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "Avg Answer Score", key: "AvgAnswerScore", higherIsBetter: true, format: (v) => v.toFixed(1) },
  { label: "Avg Latency", key: "AvgLatencyMs", higherIsBetter: false, format: (v) => `${Math.round(v)}ms` },
  { label: "Avg Prompt Tok", key: "AvgPromptTokens", higherIsBetter: false, format: (v) => Math.round(v).toString() },
  { label: "Avg Comp Tok", key: "AvgCompletionTokens", higherIsBetter: false, format: (v) => Math.round(v).toString() },
]

function getMetric(run: RunSummary, key: string): number | undefined {
  const metrics = run.metrics as Record<string, any> | null
  if (!metrics) return undefined
  const parts = key.split(".")
  let val: any = metrics
  for (const part of parts) {
    val = val?.[part]
    if (val == null) return undefined
  }
  return typeof val === "number" ? val : undefined
}

export function MetricsTable({ base, targets }: MetricsTableProps) {
  const allRuns = [base, ...targets]

  const bestValues = useMemo(() => {
    const best: Record<string, { value: number; index: number }> = {}
    for (const row of METRICS) {
      for (let i = 0; i < allRuns.length; i++) {
        const v = getMetric(allRuns[i], row.key)
        if (v == null) continue
        if (!best[row.key] || (row.higherIsBetter ? v > best[row.key].value : v < best[row.key].value)) {
          best[row.key] = { value: v, index: i }
        }
      }
    }
    return best
  }, [allRuns, base, targets])

  const getDeltaColor = (row: MetricRow, baseVal: number | undefined, compVal: number) => {
    if (baseVal == null) return "text-muted-foreground"
    if (row.higherIsBetter === false && (row.key === "AvgPromptTokens" || row.key === "AvgCompletionTokens")) {
      return "text-muted-foreground"
    }
    const diff = compVal - baseVal
    const isImprovement = row.higherIsBetter ? diff > 0 : diff < 0
    if (Math.abs(diff) < 0.001) return "text-muted-foreground"
    return isImprovement ? "text-green-600" : "text-red-600"
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Metric</TableHead>
          <TableHead className="bg-muted/30">Base ({base.tag})</TableHead>
          {targets.map((t) => (
            <TableHead key={t.id}>{t.tag} vs Base</TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {METRICS.map((row) => {
          const baseVal = getMetric(base, row.key)
          return (
            <TableRow key={row.key}>
              <TableCell className="font-medium">{row.label}</TableCell>
              <TableCell className="bg-muted/30 font-semibold">
                {baseVal != null ? row.format(baseVal) : "—"}
              </TableCell>
              {targets.map((t) => {
                const compVal = getMetric(t, row.key)
                const delta = baseVal != null && compVal != null ? compVal - baseVal : null
                const isBest = bestValues[row.key]?.index === allRuns.indexOf(t)
                return (
                  <TableCell key={t.id} className={cn(isBest && "font-semibold")}>
                    {compVal != null ? row.format(compVal) : "—"}
                    {delta != null && compVal != null && (
                      <span className={cn("ml-1.5 text-xs", getDeltaColor(row, baseVal!, compVal))}>
                        {delta >= 0 ? "+" : ""}
                        {row.higherIsBetter ? delta.toFixed(2) : (row.key === "AvgLatencyMs" ? `${Math.round(delta)}ms` : Math.round(delta).toString())}
                        {" "}
                        {delta > 0 ? (row.higherIsBetter ? "↑" : "↓") : delta < 0 ? (row.higherIsBetter ? "↓" : "↑") : ""}
                      </span>
                    )}
                  </TableCell>
                )
              })}
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
