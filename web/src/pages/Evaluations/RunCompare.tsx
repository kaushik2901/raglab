import { Link, useParams, useSearchParams } from "react-router-dom"
import { useCompareRuns } from "@/hooks/useEvaluations"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { MetricsTable } from "@/components/MetricsTable"
import { RiArrowLeftLine } from "@remixicon/react"

export default function RunCompare() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const compareTo = searchParams.getAll("compare_to")

  const { data, isLoading } = useCompareRuns(id ?? null, compareTo)

  if (isLoading || !data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-8 w-48" />
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    )
  }

  const runs = data.runs
  const base = runs[id!]
  const targets = compareTo.map((tid) => runs[tid]).filter(Boolean)

  if (!base) {
    return (
      <div className="text-center py-12">
        <p className="text-muted-foreground">Base run not found.</p>
        <Button variant="outline" className="mt-4" asChild>
          <Link to="/evaluations">Back to evaluations</Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" asChild className="size-8">
          <Link to="/evaluations">
            <RiArrowLeftLine className="size-4" />
          </Link>
        </Button>
        <div>
          <h2 className="text-lg font-semibold">Comparison</h2>
          <p className="text-xs text-muted-foreground">
            {targets.length + 1} runs · base: {base.tag}
          </p>
        </div>
      </div>

      <MetricsTable base={base} targets={targets} />
    </div>
  )
}
