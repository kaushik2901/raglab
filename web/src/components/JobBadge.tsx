import { Badge } from "@/components/ui/badge"
import { Spinner } from "@/components/ui/spinner"

const STATE_VARIANT: Record<string, "default" | "secondary" | "outline" | "destructive"> = {
  available: "secondary",
  running: "default",
  retrying: "default",
  completed: "outline",
  cancelled: "secondary",
  failed: "destructive",
}

interface JobBadgeProps {
  state: string
  showSpinner?: boolean
}

export function JobBadge({ state, showSpinner }: JobBadgeProps) {
  const variant = STATE_VARIANT[state] ?? "secondary"
  const isActive = state === "available" || state === "running" || state === "retrying"

  return (
    <Badge variant={variant} className="gap-1.5 whitespace-nowrap">
      {isActive && showSpinner !== false && <Spinner className="size-3" />}
      {state}
    </Badge>
  )
}
