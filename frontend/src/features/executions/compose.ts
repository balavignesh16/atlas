import type { Incident } from '@/api/types'

/**
 * There is no global GET /api/v1/executions endpoint (verified against
 * source: execution.Engine only exposes GetRecord(id) and
 * GetRecordsByIncident(id), and main.go registers no such route). A real
 * global execution log would require a backend addition; instead of
 * fabricating one, the Executions page composes a bounded, honestly-labeled
 * view from the incidents already known to the app.
 *
 * Bounded to the N most-recently-started incidents (not all of them,
 * unbounded) so this stays a deliberate, disclosed limitation rather than
 * an uncontrolled N+1 fetch as incident history grows across a long
 * session. The cap is a UX/performance choice, not a backend constraint.
 */
export const RECENT_INCIDENT_LIMIT = 25

export function selectRecentIncidents(incidents: Incident[], limit: number = RECENT_INCIDENT_LIMIT): Incident[] {
  return [...incidents]
    .sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime())
    .slice(0, limit)
}
