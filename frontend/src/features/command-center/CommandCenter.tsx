import { useOpenIncidentExecutions, useOpenIncidents } from '@/api/incidents'
import { Panel } from '@/components/ui/Panel'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { TableRowSkeleton } from '@/components/ui/Skeleton'
import { IncidentTable } from '@/features/incidents/IncidentTable'
import { sortIncidents } from '@/features/incidents/sort'
import { deriveIncidentState } from '@/features/incidents/incident-state'
import { DependencyHotspots } from './DependencyHotspots'
import { RecentExecutions } from './RecentExecutions'
import { StatusStrip } from './StatusStrip'

export function CommandCenter() {
  const { data: incidents, isPending, isError, error, refetch } = useOpenIncidents()
  const sorted = incidents ? sortIncidents(incidents) : []
  const { byIncidentId, forbidden } = useOpenIncidentExecutions(sorted)

  if (isError) {
    return <ErrorState error={error} onRetry={() => refetch()} />
  }

  return (
    <div className="flex h-full flex-col">
      {isPending ? null : <StatusStrip incidents={sorted} executionsByIncidentId={byIncidentId} />}

      <div className="flex min-h-0 flex-1 gap-4 p-4">
        <div className="min-w-0 flex-[2] overflow-y-auto rounded-md border border-border-subtle bg-surface-1">
          <div className="border-b border-border-subtle px-3 py-2 text-2xs font-semibold uppercase tracking-wide text-text-secondary">
            Open Incidents
          </div>
          {isPending ? (
            <table className="w-full">
              <tbody>
                <TableRowSkeleton columns={5} />
                <TableRowSkeleton columns={5} />
                <TableRowSkeleton columns={5} />
              </tbody>
            </table>
          ) : sorted.length === 0 ? (
            <EmptyState
              title="No open incidents"
              description="Atlas is currently reporting no active incidents."
            />
          ) : (
            <IncidentTable
              incidents={sorted}
              stateFor={(incident) => deriveIncidentState(incident, byIncidentId.get(incident.incidentId))}
            />
          )}
        </div>

        <div className="flex w-72 shrink-0 flex-col gap-4 overflow-y-auto">
          <Panel title="Recent Executions">
            <RecentExecutions executionsByIncidentId={byIncidentId} forbidden={forbidden} />
          </Panel>
          <Panel title="Dependency Hotspots">
            <DependencyHotspots />
          </Panel>
        </div>
      </div>
    </div>
  )
}
