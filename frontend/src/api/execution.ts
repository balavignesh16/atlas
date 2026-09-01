import { useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from './client'
import type { ExecutionRecord } from './types'

/**
 * POST /api/v1/remediation/{planId}/execute. Only `actionId` is sent --
 * the backend's `approver` body field is a legacy fallback used only when
 * no authenticated principal is present (verified in execution.go); a
 * real UI should never populate it, since doing so would look like the
 * frontend is asserting an identity, when the backend is the sole
 * authority on who actually executed something.
 */
export function useExecutePlan(incidentId: string, planId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (actionId: string) =>
      apiFetch<ExecutionRecord>(`/api/v1/remediation/${planId}/execute`, {
        method: 'POST',
        body: JSON.stringify({ actionId }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['incident', incidentId, 'executions'] })
    },
  })
}
