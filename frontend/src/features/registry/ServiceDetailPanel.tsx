import { Link } from '@tanstack/react-router'
import { isServiceNotFound, useService } from '@/api/registry'
import { StatusBadge } from '@/components/status/StatusBadge'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { SidePanel } from '@/components/ui/SidePanel'
import { Skeleton } from '@/components/ui/Skeleton'
import { Timestamp } from '@/components/ui/Timestamp'
import { describeConfidence, describeProvenance } from './provenance'
import { ServiceIntelligenceSection } from './ServiceIntelligenceSection'

/**
 * Reuses the Phase 3 SidePanel primitive, same "list stays mounted, detail
 * opens as a slide-over on a real deep-linkable route" pattern as Phase 4's
 * executions. Shows only the documented registry fields -- no fabricated
 * ownership/version/platform metadata (none exists).
 */
export function ServiceDetailPanel({ serviceName, onClose }: { serviceName: string | null; onClose: () => void }) {
  const { data: service, isPending, isError, error } = useService(serviceName)

  return (
    <SidePanel open={serviceName !== null} onOpenChange={(open) => !open && onClose()} title={serviceName ?? ''} subtitle="Service registry">
      {isPending ? (
        <div className="space-y-2 p-4">
          <Skeleton className="h-3.5 w-full" />
          <Skeleton className="h-3.5 w-2/3" />
        </div>
      ) : isError ? (
        isServiceNotFound(error) ? (
          <div className="p-4">
            <EmptyState title="Not in the registry" description="This service has never been observed by real telemetry." />
          </div>
        ) : (
          <div className="p-4">
            <ErrorState error={error} />
          </div>
        )
      ) : (
        <div className="space-y-4 p-4 text-xs">
          <div className="flex items-center gap-2">
            <StatusBadge status={service.status} />
            <span className="text-2xs text-text-muted">{describeProvenance(service.provenance)}</span>
          </div>

          <div className="space-y-1">
            <Row label="Evidence" value={describeConfidence(service.confidence)} />
            <Row label="First observed" value={<Timestamp value={service.firstObservedAt} />} />
            <Row label="Last observed" value={<Timestamp value={service.lastObservedAt} />} />
            {service.lastTelemetryAt ? <Row label="Last telemetry" value={<Timestamp value={service.lastTelemetryAt} />} /> : null}
            <Row label="Registered" value={<Timestamp value={service.createdAt} />} />
          </div>

          {service.status !== 'ACTIVE' ? (
            <p className="rounded-sm border border-border-subtle bg-surface-2 p-2 text-2xs text-text-muted">
              {service.status === 'STALE'
                ? 'No telemetry recently, but this service remains known to Atlas. It may simply be quiet right now.'
                : 'No telemetry for an extended period. This identity is preserved, not deleted.'}
            </p>
          ) : null}

          <Link to="/graph">
            <Button variant="secondary" size="sm">
              View dependency graph
            </Button>
          </Link>

          <div className="border-t border-border-subtle pt-4">
            <p className="mb-3 text-2xs uppercase tracking-wide text-text-muted">Intelligence</p>
            <ServiceIntelligenceSection serviceName={service.name} />
          </div>
        </div>
      )}
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
