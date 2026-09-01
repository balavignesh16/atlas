import { Link } from '@tanstack/react-router'
import { isServiceNotFound, useServiceDependencies } from '@/api/graph'
import { useOpenIncidents } from '@/api/incidents'
import { SidePanel } from '@/components/ui/SidePanel'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { Skeleton } from '@/components/ui/Skeleton'
import { StatusBadge } from '@/components/status/StatusBadge'
import { SeverityIndicator } from '@/components/status/SeverityIndicator'
import { deriveErrorRate, formatErrorRate } from './edge-metrics'
import type { DependencyEdge } from '@/api/types'

/**
 * "Open incidents affecting this service" is derived from the already-fetched
 * open-incidents list by checking the real `affectedServices` field --
 * verified against incidentmodel.Incident, not a name-similarity guess and
 * not a dedicated backend count endpoint (none exists).
 */
export function ServiceInspector({ serviceName, onClose }: { serviceName: string | null; onClose: () => void }) {
  const { data, isPending, isError, error } = useServiceDependencies(serviceName)
  const { data: openIncidents } = useOpenIncidents()

  const affectingIncidents = (openIncidents ?? []).filter((inc) => inc.affectedServices.includes(serviceName ?? ''))

  return (
    <SidePanel open={serviceName !== null} onOpenChange={(open) => !open && onClose()} title={serviceName ?? ''} subtitle="Service">
      {isPending ? (
        <div className="space-y-2 p-4">
          <Skeleton className="h-3.5 w-full" />
          <Skeleton className="h-3.5 w-5/6" />
          <Skeleton className="h-3.5 w-2/3" />
        </div>
      ) : isError ? (
        isServiceNotFound(error) ? (
          <div className="p-4">
            <EmptyState title="No dependency data" description="No dependencies have been observed for this service." />
          </div>
        ) : (
          <div className="p-4">
            <ErrorState error={error} />
          </div>
        )
      ) : (
        <div className="space-y-4 p-4 text-xs">
          {affectingIncidents.length > 0 ? (
            <div>
              <p className="mb-1.5 text-2xs uppercase tracking-wide text-text-muted">
                Open incidents ({affectingIncidents.length})
              </p>
              <ul className="space-y-1.5">
                {affectingIncidents.map((inc) => (
                  <li key={inc.incidentId}>
                    <Link
                      to="/incidents/$incidentId"
                      params={{ incidentId: inc.incidentId }}
                      className="flex items-center gap-2 rounded-sm border border-border-subtle bg-surface-2 px-2 py-1.5 hover:border-border-default"
                    >
                      <SeverityIndicator severity={inc.severity} />
                      <span className="truncate text-text-primary">{inc.title}</span>
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          <EdgeGroup title={`Incoming (${data.incoming.length})`} edges={data.incoming} directionLabel="from" />
          <EdgeGroup title={`Outgoing (${data.outgoing.length})`} edges={data.outgoing} directionLabel="to" />
        </div>
      )}
    </SidePanel>
  )
}

function EdgeGroup({ title, edges, directionLabel }: { title: string; edges: DependencyEdge[]; directionLabel: 'from' | 'to' }) {
  if (edges.length === 0) {
    return (
      <div>
        <p className="mb-1.5 text-2xs uppercase tracking-wide text-text-muted">{title}</p>
        <p className="text-text-disabled">None observed.</p>
      </div>
    )
  }

  return (
    <div>
      <p className="mb-1.5 text-2xs uppercase tracking-wide text-text-muted">{title}</p>
      <ul className="space-y-2">
        {edges.map((edge) => {
          const other = directionLabel === 'from' ? edge.source : edge.target
          const errorRate = deriveErrorRate(edge)
          return (
            <li key={`${edge.source}->${edge.target}`} className="rounded-sm border border-border-subtle bg-surface-2 p-2">
              <p className="font-medium text-text-primary">{other}</p>
              <div className="mt-1 grid grid-cols-3 gap-1 text-2xs">
                <Stat label="Calls" value={edge.call_count.toLocaleString()} />
                <Stat label="Errors" value={edge.error_count.toLocaleString()} />
                <Stat label="Rate" value={formatErrorRate(errorRate)} warn={edge.error_count > 0} />
              </div>
              <p className="mt-1 text-2xs text-text-muted">Avg latency {edge.average_duration_ms.toLocaleString()} ms</p>
              {edge.error_count > 0 ? <StatusBadge status="FAILED" label="Has errors" /> : null}
            </li>
          )
        })}
      </ul>
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
