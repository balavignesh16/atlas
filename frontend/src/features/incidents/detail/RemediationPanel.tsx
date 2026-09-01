import { isNoPlanYet, useApprovePlan, useGeneratePlan, useRejectPlan, useRemediationPlan } from '@/api/remediation'
import { useIdentity, type Identity } from '@/api/auth'
import { Panel } from '@/components/ui/Panel'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { Skeleton } from '@/components/ui/Skeleton'
import { Button } from '@/components/ui/Button'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { MutationError } from '@/components/ui/MutationError'
import { PermissionHint } from '@/components/ui/PermissionHint'
import { IdChip } from '@/components/ui/IdChip'
import { Timestamp } from '@/components/ui/Timestamp'
import { StatusBadge } from '@/components/status/StatusBadge'
import { canUse, permissionHint } from '@/lib/permissions'
import type { RemediationPlan } from '@/api/types'

/**
 * Real plan lifecycle only, verified against remediation/planner.go:
 * PROPOSED -> VALIDATED -> APPROVED / REJECTED / EXPIRED. Approve/Reject are
 * only offered while a plan is PROPOSED or VALIDATED (the exact same guard
 * the backend enforces server-side); this component never invents an
 * intermediate state of its own.
 *
 * Every control below is disabled/explained using the REAL permission set
 * from GET /api/v1/auth/me (Phase 5) when security is enabled -- never
 * hidden outright, and never gated on a stale frontend guess when security
 * is off. The backend's own 403 (surfaced via MutationError) remains the
 * actual authority regardless of what this UI shows.
 */
export function RemediationPanel({ incidentId }: { incidentId: string }) {
  const { data: plan, isPending, isError, error, refetch } = useRemediationPlan(incidentId)
  const { data: identity } = useIdentity()
  const generatePlan = useGeneratePlan(incidentId)

  if (isPending) {
    return (
      <Panel title="Remediation Plan">
        <div className="space-y-2 p-3">
          <Skeleton className="h-3.5 w-1/2" />
          <Skeleton className="h-3.5 w-full" />
          <Skeleton className="h-3.5 w-2/3" />
        </div>
      </Panel>
    )
  }

  if (isError) {
    if (isNoPlanYet(error)) {
      const canGenerate = canUse(identity, 'CREATE_PLAN')
      return (
        <Panel
          title="Remediation Plan"
          action={
            <Button
              size="sm"
              variant="secondary"
              onClick={() => generatePlan.mutate()}
              disabled={generatePlan.isPending || !canGenerate}
            >
              {generatePlan.isPending ? 'Generating…' : 'Generate Plan'}
            </Button>
          }
        >
          <div className="p-3">
            <EmptyState title="No remediation plan" description="A remediation plan has not been proposed for this incident." />
            {!canGenerate || generatePlan.isError ? (
              <div className="px-3 pb-3">
                <PermissionHint hint={permissionHint(identity, 'CREATE_PLAN')} />
                {generatePlan.isError ? <MutationError error={generatePlan.error} /> : null}
              </div>
            ) : null}
          </div>
        </Panel>
      )
    }
    return (
      <Panel title="Remediation Plan">
        <ErrorState error={error} onRetry={() => refetch()} />
      </Panel>
    )
  }

  const decidable = plan.status === 'PROPOSED' || plan.status === 'VALIDATED'

  return (
    <Panel title="Remediation Plan">
      <div className="space-y-3 p-3 text-xs">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2">
            <StatusBadge status={plan.status} />
            <StatusBadge status={plan.riskLevel} label={`${plan.riskLevel} RISK`} />
          </div>
          <IdChip value={plan.planId} truncate={6} />
        </div>

        <p className="text-text-primary">{plan.rationale}</p>

        {plan.preconditions.length > 0 ? (
          <ListBlock label="Preconditions" items={plan.preconditions} />
        ) : null}

        <div>
          <p className="text-2xs uppercase tracking-wide text-text-muted">Actions</p>
          <ul className="mt-1 space-y-2">
            {plan.actions.map((action) => (
              <li key={action.actionId} className="rounded-sm border border-border-subtle bg-surface-2 p-2">
                <div className="flex items-center justify-between gap-2">
                  <span className="font-mono text-2xs text-text-primary">{action.type}</span>
                  <StatusBadge status={action.riskLevel} label={action.riskLevel} />
                </div>
                <p className="mt-1 text-text-secondary">{action.description}</p>
                <p className="mt-1 text-2xs text-text-muted">Target: {action.targetService}</p>
                <p className="mt-0.5 text-2xs text-text-muted">Expected: {action.expectedOutcome}</p>
              </li>
            ))}
          </ul>
        </div>

        {plan.safetyWarnings.length > 0 ? (
          <div className="rounded-sm border border-status-warning/40 bg-status-warning-bg p-2">
            <p className="text-2xs font-semibold uppercase tracking-wide text-status-warning">Safety warnings</p>
            <ul className="mt-1 list-inside list-disc space-y-0.5 text-2xs text-text-secondary">
              {plan.safetyWarnings.map((w, i) => (
                <li key={i}>{w}</li>
              ))}
            </ul>
          </div>
        ) : null}

        {plan.rollbackPlan.length > 0 ? <ListBlock label="Rollback plan" items={plan.rollbackPlan} /> : null}
        {plan.verificationSteps.length > 0 ? <ListBlock label="Verification steps" items={plan.verificationSteps} /> : null}

        <ApprovalStatus plan={plan} />

        {decidable ? (
          <ApprovalControls incidentId={incidentId} planId={plan.planId} identity={identity} />
        ) : null}
      </div>
    </Panel>
  )
}

function ApprovalStatus({ plan }: { plan: RemediationPlan }) {
  if (plan.status === 'APPROVED' && plan.approval.approvedAt) {
    return (
      <p className="border-t border-border-subtle pt-2 text-2xs text-text-muted">
        Approved <Timestamp value={plan.approval.approvedAt} />
        {plan.approval.approvedBy ? ` by ${plan.approval.approvedBy}` : ''}
        {plan.approval.approvalReason ? ` — "${plan.approval.approvalReason}"` : ''}
      </p>
    )
  }
  if (plan.status === 'REJECTED' && plan.approval.rejectedAt) {
    return (
      <p className="border-t border-border-subtle pt-2 text-2xs text-text-muted">
        Rejected <Timestamp value={plan.approval.rejectedAt} />
        {plan.approval.rejectionReason ? ` — "${plan.approval.rejectionReason}"` : ''}
      </p>
    )
  }
  if (plan.status === 'EXPIRED') {
    return <p className="border-t border-border-subtle pt-2 text-2xs text-text-muted">This plan expired before a decision was made.</p>
  }
  return null
}

function ApprovalControls({
  incidentId,
  planId,
  identity,
}: {
  incidentId: string
  planId: string
  identity: Identity | undefined
}) {
  const approve = useApprovePlan(incidentId, planId)
  const reject = useRejectPlan(incidentId, planId)
  // Both approve and reject require the same real permission (verified in
  // main.go's routing: both /approve and /reject are gated by
  // security.PermissionApprovePlan), so one shared hint covers both.
  const canDecide = canUse(identity, 'APPROVE_PLAN')
  const hint = permissionHint(identity, 'APPROVE_PLAN')

  return (
    <div className="space-y-2 border-t border-border-subtle pt-3">
      <div className="flex items-center gap-2">
        <ConfirmDialog
          trigger={
            <Button variant="primary" size="sm" disabled={!canDecide}>
              Approve
            </Button>
          }
          title="Approve remediation plan?"
          description="This authorizes execution of the actions in this plan. This cannot be undone."
          confirmLabel="Approve"
          requireReasonInput
          onConfirm={(reason) => approve.mutateAsync(reason)}
          pending={approve.isPending}
        />
        <ConfirmDialog
          trigger={
            <Button variant="secondary" size="sm" disabled={!canDecide}>
              Reject
            </Button>
          }
          title="Reject remediation plan?"
          description="This plan will not be executed."
          confirmLabel="Reject"
          confirmVariant="secondary"
          requireReasonInput
          onConfirm={(reason) => reject.mutateAsync(reason)}
          pending={reject.isPending}
        />
      </div>
      <PermissionHint hint={hint} />
      {approve.isError ? <MutationError error={approve.error} /> : null}
      {reject.isError ? <MutationError error={reject.error} /> : null}
    </div>
  )
}

function ListBlock({ label, items }: { label: string; items: string[] }) {
  return (
    <div>
      <p className="text-2xs uppercase tracking-wide text-text-muted">{label}</p>
      <ul className="mt-1 list-inside list-disc space-y-0.5 text-text-secondary">
        {items.map((item, i) => (
          <li key={i}>{item}</li>
        ))}
      </ul>
    </div>
  )
}
