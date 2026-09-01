import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch, ApiError } from './client'
import type { AnalysisResult } from './types'

/**
 * GET /api/v1/incidents/{id}/analysis. A 404 here is a normal, expected
 * outcome meaning "no AI analysis has been generated for this incident" --
 * not an error to alarm the operator with.
 */
export function useIncidentAnalysis(incidentId: string) {
  return useQuery({
    queryKey: ['incident', incidentId, 'analysis'],
    queryFn: () => apiFetch<AnalysisResult>(`/api/v1/incidents/${incidentId}/analysis`),
    retry: false,
  })
}

export function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

/**
 * POST /api/v1/incidents/{id}/analyze. Phase 2 originally left this
 * unwired because the endpoint was dispatched unauthenticated at the time
 * (a pre-existing routing defect, verified against main.go). That defect
 * was fixed (the real, already-complete HandlePostAnalyze handler is now
 * correctly reached and gated behind the same PermissionView every sibling
 * read/insight endpoint uses), so this mutation is now safe to expose.
 * Uses the real FakeProvider unless a real provider is ever configured;
 * never fabricates a result client-side.
 */
export function useGenerateAnalysis(incidentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => apiFetch<AnalysisResult>(`/api/v1/incidents/${incidentId}/analyze`, { method: 'POST' }),
    onSuccess: (data) => {
      queryClient.setQueryData(['incident', incidentId, 'analysis'], data)
    },
  })
}
