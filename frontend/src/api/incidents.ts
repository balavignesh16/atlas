import { useQueries, useQuery } from '@tanstack/react-query'
import { apiFetch, ForbiddenError } from './client'
import type { ExecutionRecord, Incident } from './types'

export function useOpenIncidents() {
  return useQuery({
    queryKey: ['incidents', 'open'],
    queryFn: () => apiFetch<Incident[] | null>('/api/v1/incidents/open'),
    select: (data) => data ?? [],
    refetchInterval: 5000,
  })
}

export function useAllIncidents() {
  return useQuery({
    queryKey: ['incidents', 'all'],
    queryFn: () => apiFetch<Incident[] | null>('/api/v1/incidents'),
    select: (data) => data ?? [],
    refetchInterval: 15000,
  })
}

/**
 * `refetchInterval` is an explicit override (the Incident Detail page
 * computes it from real incident + execution state -- see
 * IncidentDetailPage's `computePollMs` -- 5s while OPEN/EXECUTING, 2s while
 * VERIFYING, stopped once terminal); every other caller omits it and gets
 * the previous unpolled behavior unchanged.
 */
export function useIncident(incidentId: string | undefined, refetchInterval?: number | false) {
  return useQuery({
    queryKey: ['incident', incidentId],
    queryFn: () => apiFetch<Incident>(`/api/v1/incidents/${incidentId}`),
    enabled: incidentId !== undefined,
    ...(refetchInterval !== undefined ? { refetchInterval } : {}),
  })
}

/**
 * Executions for a single incident (READ_AUDIT-gated). A 403 here is a real,
 * expected outcome for principals without READ_AUDIT (e.g. OPERATOR) -- it
 * is treated as "no data available", never surfaced as a page-breaking
 * error, per the "403 must not destroy the whole application" rule.
 */
export function useIncidentExecutions(incidentId: string, refetchInterval?: number | false) {
  return useQuery({
    queryKey: ['incident', incidentId, 'executions'],
    queryFn: () => apiFetch<ExecutionRecord[] | null>(`/api/v1/incidents/${incidentId}/executions`),
    select: (data) => data ?? [],
    retry: (failureCount, error) => !(error instanceof ForbiddenError) && failureCount < 2,
    ...(refetchInterval !== undefined ? { refetchInterval } : {}),
  })
}

/**
 * Execution state for every currently-open incident, fetched in parallel.
 * This is the one place Phase 1 does an N+1 fetch -- deliberately bounded to
 * "currently open incidents" (a small set at this project's scale, and the
 * exact set already rendered in the Command Center's own table), not the
 * full incident history. See the Phase 1 report for why the full Incidents
 * page does NOT do this same overlay.
 */
export function useOpenIncidentExecutions(incidents: Incident[]) {
  const results = useQueries({
    queries: incidents.map((incident) => ({
      queryKey: ['incident', incident.incidentId, 'executions'],
      queryFn: () => apiFetch<ExecutionRecord[] | null>(`/api/v1/incidents/${incident.incidentId}/executions`),
      select: (data: ExecutionRecord[] | null) => data ?? [],
      retry: (failureCount: number, error: unknown) => !(error instanceof ForbiddenError) && failureCount < 2,
      refetchInterval: 5000,
    })),
  })

  const byIncidentId = new Map<string, ExecutionRecord[]>()
  let forbidden = false
  results.forEach((result, index) => {
    const incidentId = incidents[index]?.incidentId
    if (!incidentId) return
    if (result.data) byIncidentId.set(incidentId, result.data)
    if (result.error instanceof ForbiddenError) forbidden = true
  })

  return {
    byIncidentId,
    forbidden,
    isLoading: results.some((r) => r.isLoading),
  }
}
