import { Link } from '@tanstack/react-router'
import { useServiceIntelligence } from '@/api/registry'
import { SeverityIndicator } from '@/components/status/SeverityIndicator'
import { StatusBadge } from '@/components/status/StatusBadge'
import { ErrorState } from '@/components/ui/ErrorState'
import { Skeleton } from '@/components/ui/Skeleton'
import { Timestamp } from '@/components/ui/Timestamp'
import type { ServiceIntelligenceDependency, ServiceIntelligenceIncident } from '@/api/types'

/**
 * Phase 7D: composed, read-only per-service view assembled at request time
 * from the registry, the live dependency graph, and incident history --
 * pure transcription of GET /api/v1/services/{name}/intelligence, no
 * client-side computation, no generated summaries. Reuses the Phase 3
 * ServiceInspector.tsx dependency-list pattern (plain muted text for an
 * empty group, not the full-page EmptyState) since this renders inside an
 * already-open side panel alongside the registry fields above it.
 *
 * Only rendered once ServiceDetailPanel's own useService(name) query has
 * already resolved -- so registry.known is always true here in practice,
 * and a 404 from this endpoint would be a genuine anomaly, not a normal
 * "nothing observed yet" state (each subsection handles that independently).
 */
export function ServiceIntelligenceSection({ serviceName }: { serviceName: string }) {
  const { data, isPending, isError, error } = useServiceIntelligence(serviceName)

  if (isPending) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-3.5 w-full" />
        <Skeleton className="h-3.5 w-2/3" />
      </div>
    )
  }

  if (isError) {
    return <ErrorState error={error} />
  }

  return (
    <div className="space-y-4">
      <DependencyGroup title="Incoming dependencies" edges={data.dependencies.incoming} />
      <DependencyGroup title="Outgoing dependencies" edges={data.dependencies.outgoing} />
      <IncidentGroup incidents={data.relevantIncidents} />
    </div>
  )
}

function DependencyGroup({ title, edges }: { title: string; edges: ServiceIntelligenceDependency[] }) {
  return (
    <div>
      <p className="mb-1.5 text-2xs uppercase tracking-wide text-text-muted">
        {title} ({edges.length})
      </p>
      {edges.length === 0 ? (
        <p className="text-text-disabled">No dependencies currently observed.</p>
      ) : (
        <ul className="space-y-2">
          {edges.map((edge) => (
            <li key={edge.service} className="rounded-sm border border-border-subtle bg-surface-2 p-2">
              <p className="font-medium text-text-primary">{edge.service}</p>
              <div className="mt-1 grid grid-cols-3 gap-1 text-2xs">
                <Stat label="Calls" value={edge.callCount.toLocaleString()} />
                <Stat label="Errors" value={edge.errorCount.toLocaleString()} warn={edge.errorCount > 0} />
                <Stat label="Avg latency" value={`${edge.averageDurationMs.toLocaleString()} ms`} />
              </div>
              <p className="mt-1 text-2xs text-text-muted">
                Last observed <Timestamp value={edge.lastObserved} />
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function IncidentGroup({ incidents }: { incidents: ServiceIntelligenceIncident[] }) {
  return (
    <div>
      <p className="mb-1.5 text-2xs uppercase tracking-wide text-text-muted">Recent incidents ({incidents.length})</p>
      {incidents.length === 0 ? (
        <p className="text-text-disabled">No recorded incidents.</p>
      ) : (
        <ul className="space-y-1.5">
          {incidents.map((inc) => (
            <li key={inc.incidentId}>
              <Link
                to="/incidents/$incidentId"
                params={{ incidentId: inc.incidentId }}
                className="flex items-center justify-between gap-2 rounded-sm border border-border-subtle bg-surface-2 px-2 py-1.5 hover:border-border-default"
              >
                <span className="flex min-w-0 items-center gap-2">
                  <SeverityIndicator severity={inc.severity} />
                  <span className="truncate text-text-primary">{inc.title}</span>
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  <StatusBadge status={inc.status} />
                  <Timestamp value={inc.startedAt} className="text-2xs text-text-muted" />
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function Stat({ label, value, warn }: { label: string; value: string; warn?: boolean }) {
  return (
    <div className="flex flex-col">
      <span className="text-text-disabled">{label}</span>
      <span className={warn ? 'font-mono text-status-critical' : 'font-mono text-text-secondary'}>{value}</span>
    </div>
  )
}
