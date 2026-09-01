import { useGraphEdges } from '@/api/graph'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton } from '@/components/ui/Skeleton'

/**
 * Real error_count/call_count from GET /api/v1/graph/edges, aggregated per
 * target service. No health score, risk score, or trend -- those don't
 * exist in the API and are not fabricated here.
 */
export function DependencyHotspots() {
  const { data: edges, isPending, isError } = useGraphEdges()

  if (isPending) {
    return (
      <div className="space-y-2 px-3 py-2">
        <Skeleton className="h-3.5 w-full" />
        <Skeleton className="h-3.5 w-full" />
        <Skeleton className="h-3.5 w-3/4" />
      </div>
    )
  }

  if (isError) {
    return <EmptyState title="Graph data unavailable" />
  }

  const errorEdges = edges.filter((edge) => edge.error_count > 0)
  if (errorEdges.length === 0) {
    return <EmptyState title="No error edges observed" />
  }

  const byService = new Map<string, number>()
  for (const edge of errorEdges) {
    byService.set(edge.target, (byService.get(edge.target) ?? 0) + edge.error_count)
  }

  const sorted = Array.from(byService.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, 5)

  return (
    <ul className="divide-y divide-border-subtle">
      {sorted.map(([service, errorCount]) => (
        <li key={service} className="flex items-center justify-between px-3 py-2 text-xs">
          <span className="text-text-primary">{service}</span>
          <span className="font-mono text-2xs text-status-critical">
            {errorCount} error {errorCount === 1 ? 'edge' : 'edges'}
          </span>
        </li>
      ))}
    </ul>
  )
}
