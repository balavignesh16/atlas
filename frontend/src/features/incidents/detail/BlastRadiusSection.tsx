import { Link } from '@tanstack/react-router'
import { Panel } from '@/components/ui/Panel'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { IdChip } from '@/components/ui/IdChip'
import type { Incident } from '@/api/types'

/**
 * Only real Incident fields -- no fabricated percentages, no decorative
 * chart standing in for a graph. Phase 3 implemented a real dependency
 * graph with incident highlighting (/graph?highlight=<incidentId>), so the
 * "view in graph" link that Phase 2 deliberately omitted (the route was
 * then just a placeholder) is added here now that it leads somewhere real.
 */
export function BlastRadiusSection({ incident }: { incident: Incident }) {
  const hasBlastData = incident.affectedServices.length > 0 || incident.traceCount > 0

  return (
    <Panel
      title="Blast Radius & Correlation"
      action={
        <Link to="/graph" search={{ highlight: incident.incidentId }}>
          <Button variant="secondary" size="sm">
            View in graph
          </Button>
        </Link>
      }
    >
      <div className="grid gap-4 p-3 text-xs sm:grid-cols-2">
        <div className="space-y-2">
          {hasBlastData ? (
            <>
              <Stat label="Traces observed" value={incident.traceCount} />
              <Stat label="Failed traces" value={incident.failureCount} />
              <div>
                <p className="text-2xs uppercase tracking-wide text-text-muted">Affected services</p>
                {incident.affectedServices.length > 0 ? (
                  <ul className="mt-1 flex flex-wrap gap-1.5">
                    {incident.affectedServices.map((s) => (
                      <li key={s} className="rounded-sm border border-border-default bg-surface-2 px-1.5 py-0.5 text-2xs text-text-secondary">
                        {s}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="mt-1 text-text-muted">None recorded.</p>
                )}
              </div>
              {incident.affectedOperations.length > 0 ? (
                <div>
                  <p className="text-2xs uppercase tracking-wide text-text-muted">Affected operations</p>
                  <p className="mt-1 text-text-secondary">{incident.affectedOperations.join(', ')}</p>
                </div>
              ) : null}
              {incident.affectedEdges.length > 0 ? (
                <div>
                  <p className="text-2xs uppercase tracking-wide text-text-muted">Affected dependency edges</p>
                  <p className="mt-1 font-mono text-2xs text-text-secondary">{incident.affectedEdges.join(', ')}</p>
                </div>
              ) : null}
            </>
          ) : (
            <EmptyState title="No blast-radius data" description="No trace or service impact has been recorded for this incident." />
          )}
        </div>

        <div className="space-y-2 border-t border-border-subtle pt-3 sm:border-l sm:border-t-0 sm:pl-4 sm:pt-0">
          {incident.correlationGroupId ? (
            <>
              <div className="flex items-baseline justify-between gap-2">
                <span className="text-2xs uppercase tracking-wide text-text-muted">Correlation group</span>
                <IdChip value={incident.correlationGroupId} truncate={6} />
              </div>
              {incident.primaryIncidentId && incident.primaryIncidentId !== incident.incidentId ? (
                <div className="flex items-baseline justify-between gap-2">
                  <span className="text-2xs uppercase tracking-wide text-text-muted">Primary incident</span>
                  <Link
                    to="/incidents/$incidentId"
                    params={{ incidentId: incident.primaryIncidentId }}
                    className="font-mono text-2xs text-accent hover:underline"
                  >
                    {incident.primaryIncidentId}
                  </Link>
                </div>
              ) : null}
              {incident.relatedIncidentIds && incident.relatedIncidentIds.length > 0 ? (
                <div>
                  <p className="text-2xs uppercase tracking-wide text-text-muted">Related incidents ({incident.relatedIncidentIds.length})</p>
                  <ul className="mt-1 space-y-1">
                    {incident.relatedIncidentIds.map((id) => (
                      <li key={id}>
                        <Link
                          to="/incidents/$incidentId"
                          params={{ incidentId: id }}
                          className="font-mono text-2xs text-accent hover:underline"
                        >
                          {id}
                        </Link>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : (
                <p className="text-text-muted">No other related incidents.</p>
              )}
            </>
          ) : (
            <EmptyState title="Not correlated" description="This incident has not been grouped with any other incident." />
          )}
        </div>
      </div>
    </Panel>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-text-muted">{label}</span>
      <span className="font-mono text-text-primary">{value}</span>
    </div>
  )
}
