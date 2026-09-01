import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiFetch, ApiError } from './client'
import type { RemediationPlan } from './types'

// GET /api/v1/incidents/{id}/remediation wraps the plan with a dry-run flag
// (internal/httpapi/remediation.go's HandleGetPlanByIncident) -- verified,
// not assumed.
export type RemediationPlanResponse = RemediationPlan & { executionSupported: boolean }

// POST .../approve responds {message, plan} -- POST .../reject responds
// with the plan directly. These are genuinely asymmetric in the real
// backend (verified against source); modeled as such rather than assumed
// to match.
export interface ApproveResponse {
  message: string
  plan: RemediationPlan
}

export function useRemediationPlan(incidentId: string) {
  return useQuery({
    queryKey: ['incident', incidentId, 'remediation'],
    queryFn: () => apiFetch<RemediationPlanResponse>(`/api/v1/incidents/${incidentId}/remediation`),
    retry: false,
  })
}

/**
 * GET /api/v1/remediation/{planId} -- verified against source
 * (RemediationAPI.HandleGetPlan) to return the raw RemediationPlan with NO
 * `executionSupported` wrapper. That field only exists on the
 * incident-scoped variant above; conflating the two shapes would silently
 * lose type safety, so this is modeled as its own distinct return type.
 */
export function useRemediationPlanById(planId: string | null) {
  return useQuery({
    queryKey: ['remediation', planId],
    queryFn: () => apiFetch<RemediationPlan>(`/api/v1/remediation/${planId}`),
    enabled: planId !== null,
    retry: false,
  })
}

export function isNoPlanYet(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

export function useGeneratePlan(incidentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () =>
      apiFetch<RemediationPlan>(`/api/v1/incidents/${incidentId}/remediation/plan`, { method: 'POST' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['incident', incidentId, 'remediation'] })
    },
  })
}

export function useApprovePlan(incidentId: string, planId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (reason: string) =>
      apiFetch<ApproveResponse>(`/api/v1/remediation/${planId}/approve`, {
        method: 'POST',
        body: JSON.stringify({ reason }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['incident', incidentId, 'remediation'] })
    },
  })
}

export function useRejectPlan(incidentId: string, planId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (reason: string) =>
      apiFetch<RemediationPlan>(`/api/v1/remediation/${planId}/reject`, {
        method: 'POST',
        body: JSON.stringify({ reason }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['incident', incidentId, 'remediation'] })
    },
  })
}
