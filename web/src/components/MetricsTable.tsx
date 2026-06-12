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
  { label: "HitRate@1", key: "hit_rate_1", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "HitRate@3", key: "hit_rate_3", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "HitRate@5", key: "hit_rate_5", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "MRR", key: "mrr", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "NDCG@5", key: "ndcg_5", higherIsBetter: true, format: (v) => v.toFixed(2) },
  { label: "Avg Answer Score", key: "avg_answer_score", higherIsBetter: true, format: (v) => v.toFixed(1) },
  { label: "Avg Latency", key: "avg_latency_ms", higherIsBetter: false, format: (v) => `${Math.round(v)}ms` },
  { label: "Avg Prompt Tok", key: "avg_prompt_tokens", higherIsBetter: false, format: (v) => Math.round(v).toString() },
  { label: "Avg Comp Tok", key: "avg_completion_tokens", higherIsBetter: false, format: (v) => Math.round(v).toString() },
]

function getMetric(run: RunSummary, key: string): number | undefined {
  const metrics = run.metrics as Record<string, number> | null
  return metrics?.[key]
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

  const getDeltaColor = (row: MetricRow, baseVal: number, compVal: number) => {
    if (row.higherIsBetter === false && (row.key === "avg_prompt_tokens" || row.key === "avg_completion_tokens")) {
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
                    {delta != null && (
                      <span className={cn("ml-1.5 text-xs", getDeltaColor(row, baseVal!, compVal))}>
                        {delta >= 0 ? "+" : ""}
                        {row.higherIsBetter ? delta.toFixed(2) : (row.key === "avg_latency_ms" ? `${Math.round(delta)}ms` : Math.round(delta).toString())}
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
