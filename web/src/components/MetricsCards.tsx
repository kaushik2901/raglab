import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

interface Metric {
  label: string
  value: string | number
  subtitle?: string
}

interface MetricsCardsProps {
  metrics: Metric[]
}

export function MetricsCards({ metrics }: MetricsCardsProps) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
      {metrics.map((m) => (
        <Card key={m.label}>
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-medium text-muted-foreground">{m.label}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold">{m.value}</div>
            {m.subtitle && (
              <p className="text-xs text-muted-foreground mt-0.5">{m.subtitle}</p>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
