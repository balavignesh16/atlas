import { useQuery } from '@tanstack/react-query'
import { apiFetch } from './client'

/**
 * GET /api/v1/incidents/{id}/rca returns a richer, purpose-built shape than
 * the RootCause embedded on Incident -- notably `limitations`, a real,
 * backend-authored disclaimer ("Root cause is probabilistic and based only
 * on observed telemetry"), which is exactly the honest-uncertainty framing
 * Phase 2 needs. Verified directly against
 * internal/httpapi/incident.go's handleGetRCA.
 */
export interface RCAResponse {
  rootCause: string
  confidence: string
  score: number
  candidates: string[]
  evidenceIds: string[]
  reasoning: string[]
  limitations: string[]
}

export function useIncidentRCA(incidentId: string) {
  return useQuery({
    queryKey: ['incident', incidentId, 'rca'],
    queryFn: () => apiFetch<RCAResponse>(`/api/v1/incidents/${incidentId}/rca`),
  })
}
