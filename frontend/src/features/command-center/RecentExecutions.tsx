import { Link } from '@tanstack/react-router'
import type { ExecutionRecord } from '@/api/types'
import { StatusBadge } from '@/components/status/StatusBadge'
import { EmptyState } from '@/components/ui/EmptyState'
import { IdChip } from '@/components/ui/IdChip'
import { Timestamp } from '@/components/ui/Timestamp'

/**
 * Derived honestly from the per-incident execution records already fetched
 * for the open-incidents table above -- there is no global executions list
 * endpoint (see the Phase 1 report's API gaps section), so this is
 * necessarily scoped to executions belonging to currently open incidents,
 * not a true global history.
 */
export function RecentExecutions({
  executionsByIncidentId,
  forbidden,
}: {
  executionsByIncidentId: Map<string, ExecutionRecord[]>
  forbidden: boolean
}) {
  if (forbidden) {
    return (
      <EmptyState
        title="Execution data unavailable"
        description="Your current role does not include READ_AUDIT."
      />
    )
  }

  const all = Array.from(executionsByIncidentId.values())
    .flat()
    .sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime())
    .slice(0, 5)

  if (all.length === 0) {
    return <EmptyState title="No recent executions" />
  }

  return (
    <ul className="divide-y divide-border-subtle">
      {all.map((execution) => (
        <li key={execution.executionId} className="flex items-center justify-between gap-2 px-3 py-2">
          <div className="min-w-0">
            <Link
              to="/incidents/$incidentId"
              params={{ incidentId: execution.incidentId }}
              className="block truncate text-xs font-medium text-text-primary hover:text-accent"
            >
              {execution.service}
            </Link>
            <div className="flex items-center gap-1.5 text-2xs text-text-muted">
              <IdChip value={execution.executionId} truncate={6} />
              <Timestamp value={execution.startedAt} />
            </div>
          </div>
          <StatusBadge status={execution.verificationStatus} />
        </li>
      ))}
    </ul>
  )
}
