import { Link } from "react-router-dom"
import { useJobs } from "@/hooks/useWorkflows"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"
import { JobBadge } from "@/components/JobBadge"

const KIND_LINK: Record<string, string> = {
  preprocess: "/artifacts",
  index: "/indexes",
  eval: "/evaluations",
}

export function JobTimeline() {
  const { data: jobs, isLoading } = useJobs()

  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </div>
    )
  }

  if (!jobs || jobs.length === 0) {
    return <p className="text-sm text-muted-foreground">No recent jobs</p>
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Kind</TableHead>
          <TableHead>Tag</TableHead>
          <TableHead>State</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {jobs.slice(0, 10).map((j) => (
          <TableRow key={j.id}>
            <TableCell className="text-xs text-muted-foreground uppercase">{j.kind}</TableCell>
            <TableCell className="font-medium text-sm">
              <Link to={KIND_LINK[j.kind] ?? "#"} className="hover:underline">
                {j.tag}
              </Link>
            </TableCell>
            <TableCell>
              <JobBadge state={j.state} />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
