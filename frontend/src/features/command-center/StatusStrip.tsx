import type { ExecutionRecord, Incident } from '@/api/types'

/**
 * One line, not four giant metric cards. Every number here is a direct
 * count over real, already-fetched data.
 *
 * "Pending approval" is deliberately NOT shown here: deriving it honestly
 * would require an additional per-incident fetch of each incident's
 * remediation plan (GET /incidents/{id}/remediation) purely to check its
 * status, which is scope creep beyond Phase 1 (no approval UI exists yet).
 * It belongs in Phase 2 alongside the remediation/approval experience
 * itself, not bolted onto this strip early.
 */
export function StatusStrip({
  incidents,
  executionsByIncidentId,
}: {
  incidents: Incident[]
  executionsByIncidentId: Map<string, ExecutionRecord[]>
}) {
  const critical = incidents.filter((i) => i.severity === 'CRITICAL').length

  const allExecutions = Array.from(executionsByIncidentId.values()).flat()
  const executing = allExecutions.filter(
    (e) => e.executionStatus === 'EXECUTING' || e.executionStatus === 'PRECONDITION_CHECK',
  ).length

  return (
    <div className="flex items-center gap-1.5 border-b border-border-subtle bg-surface-1 px-4 py-2.5 text-xs">
      <Segment value={incidents.length} label={incidents.length === 1 ? 'open incident' : 'open incidents'} />
      <Divider />
      <Segment value={critical} label="critical" emphasize={critical > 0} />
      <Divider />
      <Segment value={executing} label="executing" emphasize={executing > 0} />
    </div>
  )
}

function Segment({ value, label, emphasize }: { value: number; label: string; emphasize?: boolean }) {
  return (
    <span className={emphasize ? 'font-medium text-status-critical' : 'text-text-secondary'}>
      <span className="font-mono font-semibold">{value}</span> {label}
    </span>
  )
}

function Divider() {
  return <span className="text-text-muted">·</span>
}
