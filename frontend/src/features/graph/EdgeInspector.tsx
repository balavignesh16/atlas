import type { DependencyEdge } from '@/api/types'
import { SidePanel } from '@/components/ui/SidePanel'
import { Timestamp } from '@/components/ui/Timestamp'
import { deriveErrorRate, formatErrorRate } from './edge-metrics'

/**
 * Click-to-inspect, distinct from the edge's hover tooltip: this stays open
 * while you look elsewhere on the graph, and shows the same real fields
 * plus first/last-observed timestamps the hover tooltip omits for space.
 */
export function EdgeInspector({ edge, onClose }: { edge: DependencyEdge | null; onClose: () => void }) {
  const errorRate = edge ? deriveErrorRate(edge) : null

  return (
    <SidePanel
      open={edge !== null}
      onOpenChange={(open) => !open && onClose()}
      title={edge ? `${edge.source} → ${edge.target}` : ''}
      subtitle="Dependency"
    >
      {edge ? (
        <div className="space-y-3 p-4 text-xs">
          <Row label="Source" value={<span className="font-mono">{edge.source}</span>} />
          <Row label="Target" value={<span className="font-mono">{edge.target}</span>} />
          <div className="border-t border-border-subtle pt-3">
            <Row label="Calls" value={edge.call_count.toLocaleString()} />
            <Row label="Errors" value={edge.error_count.toLocaleString()} />
            <Row label="Error rate" value={formatErrorRate(errorRate)} />
            <Row label="Avg latency" value={`${edge.average_duration_ms.toLocaleString()} ms`} />
          </div>
          <div className="border-t border-border-subtle pt-3">
            <Row label="First observed" value={<Timestamp value={edge.first_observed} />} />
            <Row label="Last observed" value={<Timestamp value={edge.last_observed} />} />
          </div>
          {Object.keys(edge.status_counts ?? {}).length > 0 ? (
            <div className="border-t border-border-subtle pt-3">
              <p className="mb-1.5 text-2xs uppercase tracking-wide text-text-muted">Status codes observed</p>
              <ul className="space-y-1">
                {Object.entries(edge.status_counts).map(([status, count]) => (
                  <Row key={status} label={status} value={count.toLocaleString()} />
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      ) : null}
    </SidePanel>
  )
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-0.5">
      <span className="text-text-muted">{label}</span>
      <span className="text-text-primary">{value}</span>
    </div>
  )
}
