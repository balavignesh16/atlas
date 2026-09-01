import { Link } from '@tanstack/react-router'
import { ForbiddenError } from '@/api/client'
import { useExecutePlan } from '@/api/execution'
import { useIncidentExecutions } from '@/api/incidents'
import { useIdentity, type Identity } from '@/api/auth'
import type { RemediationPlanResponse } from '@/api/remediation'
import { StatusBadge } from '@/components/status/StatusBadge'
import { Button } from '@/components/ui/Button'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { IdChip } from '@/components/ui/IdChip'
import { MutationError } from '@/components/ui/MutationError'
import { Panel } from '@/components/ui/Panel'
import { PermissionHint } from '@/components/ui/PermissionHint'
import { Skeleton } from '@/components/ui/Skeleton'
import { Timestamp } from '@/components/ui/Timestamp'
import { canUse, permissionHint } from '@/lib/permissions'

/**
 * SAFETY-CRITICAL UI. ExecutionStatus and VerificationStatus are two
 * genuinely independent fields on ExecutionRecord (verified against
 * internal/execution's model) -- an action can be EXECUTED and still be
 * VERIFYING, FAILED, VERIFICATION_TIMEOUT, or NOT_REQUIRED. This panel
 * always renders them as two separate, separately-badged rows and never
 * collapses them into one combined "success/fail" pill. Execution never
 * implies verification.
 */
export function ExecutionPanel({
  incidentId,
  plan,
}: {
  incidentId: string
  plan: RemediationPlanResponse | null
}) {
  const { data, isPending, isError, error, refetch } = useIncidentExecutions(incidentId)
  const { data: identity } = useIdentity()

  if (isPending) {
    return (
      <Panel title="Execution">
        <div className="space-y-2 p-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      </Panel>
    )
  }

  if (isError) {
    if (error instanceof ForbiddenError) {
      return (
        <Panel title="Execution">
          <EmptyState
            title="Execution records unavailable"
            description={
              <>
                Viewing execution history requires <span className="font-mono">READ_AUDIT</span>.
                {identity?.role ? ` Your current role: ${identity.role}.` : null}
              </>
            }
          />
        </Panel>
      )
    }
    return (
      <Panel title="Execution">
        <ErrorState error={error} onRetry={() => refetch()} />
      </Panel>
    )
  }

  const canExecute = plan !== null && plan.status === 'APPROVED'

  return (
    <Panel title="Execution">
      <div className="space-y-3 p-3 text-xs">
        {data.length === 0 ? (
          <EmptyState title="No executions" description="No actions have been executed for this incident yet." />
        ) : (
          <ul className="space-y-2">
            {[...data]
              .sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime())
              .map((record) => (
                <li key={record.executionId} className="rounded-sm border border-border-subtle bg-surface-2 p-2.5">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-2xs text-text-primary">{record.action}</span>
                    <div className="flex items-center gap-1.5">
                      <IdChip value={record.executionId} truncate={6} />
                      <Link
                        to="/executions/$executionId"
                        params={{ executionId: record.executionId }}
                        className="text-2xs text-accent hover:underline"
                      >
                        Details →
                      </Link>
                    </div>
                  </div>
                  <p className="mt-1 text-2xs text-text-muted">
                    {record.service} · started <Timestamp value={record.startedAt} />
                    {record.finishedAt ? (
                      <>
                        {' '}
                        · finished <Timestamp value={record.finishedAt} />
                      </>
                    ) : null}
                  </p>

                  <div className="mt-2 grid grid-cols-2 gap-2">
                    <StatusRow label="Execution" status={record.executionStatus} />
                    <StatusRow label="Verification" status={record.verificationStatus} />
                  </div>

                  {record.message ? <p className="mt-2 text-text-secondary">{record.message}</p> : null}
                  {record.error ? <p className="mt-2 text-status-critical">{record.error}</p> : null}
                </li>
              ))}
          </ul>
        )}

        {plan ? (
          <ExecuteControls incidentId={incidentId} plan={plan} canExecute={canExecute} identity={identity} />
        ) : null}
      </div>
    </Panel>
  )
}

function StatusRow({ label, status }: { label: string; status: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-2xs uppercase tracking-wide text-text-muted">{label}</span>
      <StatusBadge status={status} />
    </div>
  )
}

function ExecuteControls({
  incidentId,
  plan,
  canExecute,
  identity,
}: {
  incidentId: string
  plan: RemediationPlanResponse
  canExecute: boolean
  identity: Identity | undefined
}) {
  const execute = useExecutePlan(incidentId, plan.planId)
  const hasExecutePermission = canUse(identity, 'EXECUTE')

  // NOTE: `plan.executionSupported` is NOT used here. Verified live against
  // a running backend: the field is a hardcoded `false` constant in
  // internal/httpapi/remediation.go's HandleGetPlanByIncident (its own
  // comment calls it a "dry-run representation"), unrelated to whether
  // POST /execute actually works -- executing a real approved plan against
  // the live backend succeeded despite this field reading false. Gating
  // the control on it would hide a working action behind a stale flag; the
  // guard's own real rejection (ErrExecutionDisabled, fingerprint mismatch,
  // not allowlisted, etc.) is what actually governs this, and surfaces
  // through MutationError below if it fires.
  if (!canExecute) {
    return (
      <p className="border-t border-border-subtle pt-2 text-2xs text-text-muted">
        Execution is available once this plan is APPROVED (currently {plan.status}).
      </p>
    )
  }

  return (
    <div className="space-y-2 border-t border-border-subtle pt-3">
      {plan.actions.map((action) => (
        <div key={action.actionId} className="flex items-center justify-between gap-2">
          <span className="truncate font-mono text-2xs text-text-secondary">{action.type}</span>
          <ConfirmDialog
            trigger={
              <Button variant="primary" size="sm" disabled={!hasExecutePermission}>
                Execute
              </Button>
            }
            title="Execute this action?"
            description={`This will run "${action.type}" against ${action.targetService}. This cannot be undone.`}
            confirmLabel="Execute"
            onConfirm={() => execute.mutateAsync(action.actionId)}
            pending={execute.isPending}
          />
        </div>
      ))}
      <PermissionHint hint={permissionHint(identity, 'EXECUTE')} />
      {execute.isError ? <MutationError error={execute.error} /> : null}
    </div>
  )
}
