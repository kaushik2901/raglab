import { useMemo } from "react"
import { Link } from "react-router-dom"
import { useArtifacts } from "@/hooks/useArtifacts"
import { useIndexes } from "@/hooks/useIndexes"
import { useEvalRuns } from "@/hooks/useEvaluations"
import { useDatasets } from "@/hooks/useDatasets"
import { useJobs } from "@/hooks/useWorkflows"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { JobBadge } from "@/components/JobBadge"
import { RiArrowRightLine, RiFolderLine, RiSearch2Line, RiLineChartLine, RiDatabase2Line } from "@remixicon/react"
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts"

export default function Dashboard() {
  const artifacts = useArtifacts()
  const indexes = useIndexes()
  const evalRuns = useEvalRuns(1, 20)
  const datasets = useDatasets()
  const jobs = useJobs()

  const runningJobs = (jobs.data ?? []).filter((j) => j.state === "running" || j.state === "retrying" || j.state === "available")
  const recentJobs = (jobs.data ?? []).slice(0, 8)

  const trendData = useMemo(() => {
    const runs = [...(evalRuns.data?.runs ?? [])].reverse()
    return runs.map((r) => {
      const m = (r.metrics as Record<string, any>) ?? {}
      return {
        tag: r.tag.length > 14 ? r.tag.slice(0, 14) + "..." : r.tag,
        hr: m.HitRate?.["5"] ?? null,
        mrr: m.MRR ?? null,
        ndcg: m.NDCG?.["5"] ?? null,
        score: m.AvgAnswerScore ?? null,
        date: r.created_at ? new Date(r.created_at).toLocaleDateString("en-US", { month: "short", day: "numeric" }) : "",
      }
    })
  }, [evalRuns.data])

  const isLoading = artifacts.isLoading || indexes.isLoading || evalRuns.isLoading || datasets.isLoading || jobs.isLoading

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-28 w-full rounded-lg" />
          ))}
        </div>
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <Skeleton className="h-72 w-full rounded-lg" />
          <Skeleton className="h-72 w-full rounded-lg" />
        </div>
      </div>
    )
  }

  const statCards = [
    {
      label: "Artifacts",
      value: artifacts.data.length,
      icon: RiFolderLine,
      color: "#7c3aed",
      to: "/artifacts",
    },
    {
      label: "Indexes",
      value: indexes.data.length,
      icon: RiSearch2Line,
      color: "#2563eb",
      to: "/indexes",
    },
    {
      label: "Evaluation Runs",
      value: evalRuns.data?.total ?? 0,
      icon: RiLineChartLine,
      color: "#059669",
      to: "/evaluations",
    },
    {
      label: "Datasets",
      value: (datasets.data ?? []).length,
      icon: RiDatabase2Line,
      color: "#d97706",
      to: "/datasets",
    },
  ]

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Dashboard</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Overview of your RAG pipeline — artifacts, indexes, evaluations, and active jobs.
        </p>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        {statCards.map((s) => (
          <Link key={s.label} to={s.to}>
            <Card className="hover:border-primary/40 hover:shadow-sm transition-all duration-200 cursor-pointer h-full">
              <CardContent className="p-5 flex flex-col gap-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
                    {s.label}
                  </span>
                  <div className="flex size-8 items-center justify-center rounded-md" style={{ backgroundColor: `${s.color}18` }}>
                    <s.icon className="size-4" style={{ color: s.color }} />
                  </div>
                </div>
                <div className="flex items-baseline gap-1">
                  <span className="text-3xl font-bold tracking-tight">{s.value}</span>
                </div>
              </CardContent>
            </Card>
          </Link>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card className="lg:col-span-2">
          <CardHeader className="pb-4">
            <CardTitle className="text-sm font-medium">Evaluation Trends</CardTitle>
          </CardHeader>
          <CardContent className="pb-6">
            {trendData.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <RiLineChartLine className="size-8 text-muted-foreground/40 mb-3" />
                <p className="text-sm text-muted-foreground">No evaluation runs yet</p>
                <Link to="/evaluations/new" className="text-sm text-primary hover:underline mt-1">
                  Run your first evaluation
                </Link>
              </div>
            ) : (
              <ResponsiveContainer width="100%" height={280}>
                <AreaChart data={trendData} margin={{ top: 5, right: 10, left: -20, bottom: 0 }}>
                  <defs>
                    <linearGradient id="hrGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.2} />
                      <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0} />
                    </linearGradient>
                    <linearGradient id="ndcgGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="var(--chart-3)" stopOpacity={0.2} />
                      <stop offset="100%" stopColor="var(--chart-3)" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
                  <XAxis dataKey="date" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} className="fill-muted-foreground" />
                  <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} domain={[0, 1]} className="fill-muted-foreground" />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: "var(--popover)",
                      border: "1px solid var(--border)",
                      borderRadius: "0.5rem",
                      fontSize: "0.8rem",
                    }}
                  />
                  <Area
                    type="monotone"
                    dataKey="hr"
                    name="HitRate@5"
                    stroke="var(--chart-1)"
                    strokeWidth={2}
                    fill="url(#hrGrad)"
                    connectNulls
                  />
                  <Area
                    type="monotone"
                    dataKey="ndcg"
                    name="NDCG@5"
                    stroke="var(--chart-3)"
                    strokeWidth={2}
                    fill="url(#ndcgGrad)"
                    connectNulls
                  />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-4">
            <CardTitle className="text-sm font-medium">Active Jobs</CardTitle>
          </CardHeader>
          <CardContent>
            {runningJobs.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                <div className="flex size-8 items-center justify-center rounded-full bg-muted mb-3">
                  <RiArrowRightLine className="size-4 text-muted-foreground/60" />
                </div>
                <p className="text-sm text-muted-foreground">No active jobs</p>
                <p className="text-xs text-muted-foreground mt-0.5">All pipelines are idle</p>
              </div>
            ) : (
              <div className="space-y-1">
                {runningJobs.map((j) => (
                  <div key={j.id} className="flex items-center justify-between py-2 border-b border-border last:border-0">
                    <div className="min-w-0">
                      <p className="text-sm font-medium truncate">{j.tag}</p>
                      <p className="text-xs text-muted-foreground capitalize">{j.kind}</p>
                    </div>
                    <JobBadge state={j.state} />
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-4">
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm font-medium">Recent Jobs</CardTitle>
            <span className="text-xs text-muted-foreground">
              {runningJobs.length > 0 ? `${runningJobs.length} active` : "All complete"}
            </span>
          </div>
        </CardHeader>
        <CardContent className="pb-4">
          {recentJobs.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-center">
              <p className="text-sm text-muted-foreground">No jobs recorded yet</p>
            </div>
          ) : (
            <div className="space-y-1">
              {recentJobs.map((j) => (
                <div key={j.id} className="flex items-center gap-4 py-2.5 px-2 rounded-md hover:bg-muted/50 transition-colors">
                  <JobBadge state={j.state} />
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{j.tag}</span>
                      <span className="text-xs text-muted-foreground px-1.5 py-0.5 rounded bg-muted capitalize">
                        {j.kind}
                      </span>
                    </div>
                  </div>
                  <span className="text-xs text-muted-foreground shrink-0">
                    {j.created_at ? new Date(j.created_at).toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit" }) : ""}
                  </span>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
