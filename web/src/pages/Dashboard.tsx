import { Link } from "react-router-dom"
import { useArtifacts } from "@/hooks/useArtifacts"
import { useIndexes } from "@/hooks/useIndexes"
import { useEvalRuns } from "@/hooks/useEvaluations"
import { useDatasets } from "@/hooks/useDatasets"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { JobTimeline } from "@/components/JobTimeline"

export default function Dashboard() {
  const artifacts = useArtifacts()
  const indexes = useIndexes()
  const evalRuns = useEvalRuns(1, 5)
  const datasets = useDatasets()

  const isLoading = artifacts.isLoading || indexes.isLoading || evalRuns.isLoading || datasets.isLoading

  if (isLoading) {
    return (
      <div className="space-y-6">
        <h2 className="text-lg font-semibold">Dashboard</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
      </div>
    )
  }

  const statCards = [
    { label: "Artifacts", count: artifacts.data.length, to: "/artifacts" },
    { label: "Indexes", count: indexes.data.length, to: "/indexes" },
    { label: "Eval Runs", count: evalRuns.data?.total ?? 0, to: "/evaluations" },
    { label: "Datasets", count: (datasets.data ?? []).length, to: "/datasets" },
  ]

  return (
    <div className="space-y-8">
      <h2 className="text-lg font-semibold">Dashboard</h2>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        {statCards.map((s) => (
          <Link key={s.label} to={s.to}>
            <Card className="hover:border-primary/50 transition-colors cursor-pointer h-full">
              <CardHeader className="pb-2">
                <CardTitle className="text-xs font-medium text-muted-foreground">{s.label}</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-semibold">{s.count}</div>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div className="space-y-3">
          <h3 className="text-sm font-medium">Recent Jobs</h3>
          <JobTimeline />
        </div>
        <div className="space-y-3">
          <h3 className="text-sm font-medium">Recent Evaluations</h3>
          {(evalRuns.data?.runs ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">No evaluations yet</p>
          ) : (
            <div className="space-y-2">
              {(evalRuns.data?.runs ?? []).map((run) => {
                const metrics = run.metrics as Record<string, any> | null
                return (
                  <Link key={run.id} to={`/evaluations/${run.id}`}>
                    <Card className="hover:border-primary/50 transition-colors">
                      <CardContent className="p-3 flex justify-between items-center">
                        <span className="text-sm font-medium">{run.tag}</span>
                        <span className="text-xs text-muted-foreground">
                          {metrics?.HitRate?.["5"] != null ? `HR@5: ${(metrics.HitRate["5"] as number).toFixed(2)}` : "—"}
                        </span>
                      </CardContent>
                    </Card>
                  </Link>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
