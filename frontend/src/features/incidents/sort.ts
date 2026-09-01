import type { Incident } from '@/api/types'

const SEVERITY_RANK: Record<Incident['severity'], number> = { CRITICAL: 0, WARNING: 1, INFO: 2 }

/** Sort order per Phase 1 spec: 1. CRITICAL, 2. WARNING, 3. lower severity,
 * 4. newest first within the same severity. No fabricated "priority score"
 * -- this compares only the real severity and startedAt fields. */
export function sortIncidents(incidents: Incident[]): Incident[] {
  return [...incidents].sort((a, b) => {
    const severityDiff = SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity]
    if (severityDiff !== 0) return severityDiff
    return new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
  })
}
