import { ApiError, ForbiddenError, UnauthorizedError } from '@/api/client'

/**
 * Inline error for a single action (approve/reject/execute/generate-plan),
 * scoped to that control -- never a page-breaking error.
 *
 * Distinguishes two genuinely different backend outcomes that both arrive
 * as HTTP 403, verified against source:
 *  - RBAC rejection (internal/security's RequirePermission) -- body contains
 *    "permission: <NAME>", extracted by the API client as
 *    `requiredPermission`.
 *  - Guard/business-logic rejection (internal/execution/guard.go, e.g.
 *    "plan is not in APPROVED status") -- same HTTP status, unrelated to
 *    RBAC, and must not be mislabeled as a permissions issue.
 */
export function MutationError({ error }: { error: unknown }) {
  if (error instanceof UnauthorizedError) {
    return <InlineError>Your API key is invalid or has expired.</InlineError>
  }

  if (error instanceof ForbiddenError) {
    if (error.requiredPermission) {
      return (
        <InlineError>
          Requires <span className="font-mono">{error.requiredPermission}</span>.
        </InlineError>
      )
    }
    return <InlineError>{error.message}</InlineError>
  }

  if (error instanceof ApiError) {
    return <InlineError>{error.message}</InlineError>
  }

  return <InlineError>{error instanceof Error ? error.message : 'The action could not be completed.'}</InlineError>
}

function InlineError({ children }: { children: React.ReactNode }) {
  return <p className="text-2xs text-status-critical">{children}</p>
}
