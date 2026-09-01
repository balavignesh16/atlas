import { useQueries, useQuery } from '@tanstack/react-query'
import { apiFetch, ForbiddenError } from './client'
import { useAllIncidents } from './incidents'
import type { ExecutionRecord } from './types'
import { RECENT_INCIDENT_LIMIT, selectRecentIncidents } from '@/features/executions/compose'
import { sortExecutionsByStartedAt } from '@/features/executions/filter'

/** GET /api/v1/executions/{id} -- the only single-execution read endpoint
 * that exists (verified against source). */
export function useExecution(executionId: string | null) {
  return useQuery({
    queryKey: ['execution', executionId],
    queryFn: () => apiFetch<ExecutionRecord>(`/api/v1/executions/${executionId}`),
    enabled: executionId !== null,
    retry: false,
  })
}

/**
 * Composes a bounded "recent executions" view from the RECENT_INCIDENT_LIMIT
 * most-recently-started incidents (see compose.ts for why no global
 * endpoint exists), fanning out to each incident's real
 * GET /incidents/{id}/executions using the exact same query key
 * (['incident', id, 'executions']) the Command Center and Incident Detail
 * already use -- so this never double-fetches data those pages already
 * hold in cache.
 *
 * This is NOT a full execution history. It is disclosed as such in the UI
 * (ExecutionsPage shows the incident count considered).
 */
export function useRecentExecutions() {
  const { data: incidents, isPending: incidentsPending, isError: incidentsError, error: incidentsErrorValue } = useAllIncidents()
  const consideredIncidents = incidents ? selectRecentIncidents(incidents) : []

  const results = useQueries({
    queries: consideredIncidents.map((incident) => ({
      queryKey: ['incident', incident.incidentId, 'executions'],
      queryFn: () => apiFetch<ExecutionRecord[] | null>(`/api/v1/incidents/${incident.incidentId}/executions`),
      select: (data: ExecutionRecord[] | null) => data ?? [],
      retry: (failureCount: number, error: unknown) => !(error instanceof ForbiddenError) && failureCount < 2,
    })),
  })

  const forbidden = results.some((r) => r.error instanceof ForbiddenError)
  const isPending = incidentsPending || (consideredIncidents.length > 0 && results.some((r) => r.isLoading))
  const records = sortExecutionsByStartedAt(results.flatMap((r) => r.data ?? []))

  return {
    records,
    isPending,
    isError: incidentsError && !incidentsPending,
    error: incidentsErrorValue,
    forbidden,
    consideredIncidentCount: consideredIncidents.length,
    totalIncidentCount: incidents?.length ?? 0,
    limit: RECENT_INCIDENT_LIMIT,
  }
}
