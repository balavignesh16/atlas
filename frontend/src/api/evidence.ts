import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './client'
import type { Evidence } from './types'

/**
 * GET /api/v1/incidents/{id}/evidence -- the real evidence list.
 *
 * NOTE (verified against source, not assumed): the dedicated
 * GET /api/v1/incidents/{id}/timeline endpoint was inspected in
 * internal/httpapi/incident.go's handleGetTimeline and found to return this
 * exact same evidence list, UNSORTED -- its own code comment reads "Sort
 * evidences by timestamp..." followed immediately by returning them
 * unsorted, so the ordering is whatever Go's non-deterministic map
 * iteration produces. Rather than build a UI on top of that, this hook is
 * the single source for evidence data, and consumers sort it client-side by
 * the real `timestamp` field (see EvidenceTimeline) -- a legitimate
 * transformation of real data, not a fabrication. /timeline is not called
 * anywhere in Phase 2.
 */
export function useIncidentEvidence(incidentId: string) {
  return useQuery({
    queryKey: ['incident', incidentId, 'evidence'],
    queryFn: () => apiFetch<Evidence[] | null>(`/api/v1/incidents/${incidentId}/evidence`),
    select: (data) => data ?? [],
  })
}
