import clsx from 'clsx'
import type { ExecutionRecord, Incident, RemediationPlan } from '@/api/types'
import { StatusBadge } from '@/components/status/StatusBadge'

/**
 * THIS IS A CORRECTNESS-CRITICAL COMPONENT, not a decorative progress bar.
 *
 * The real backend model (verified against source, not assumed) is NOT a
 * single linear pipeline:
 *
 *   Incident.Status:      OPEN -> RESOLVED                     (independent)
 *   Incident.RCA:         set independently, any confidence     (independent)
 *   RemediationPlan.Status: PROPOSED -> VALIDATED -> APPROVED/REJECTED/EXPIRED
 *   ExecutionRecord.ExecutionStatus:   PENDING/EXECUTING/EXECUTED/FAILED/...
 *   ExecutionRecord.VerificationStatus: PENDING/VERIFYING/VERIFIED/FAILED/
 *                                       VERIFICATION_TIMEOUT/NOT_REQUIRED
 *
 * A plan may never exist (RCA too ambiguous/low-confidence for M2.6's
 * policy to allow one). An approved plan may never execute. An executed
 * action may never verify as healthy. This component never infers one
 * stage's completeness from another's -- each of Detected/Correlated/RCA
 * is its own real boolean/value check, and Plan/Execution/Verification are
 * rendered as three INDEPENDENT states after a visual branch, never
 * collapsed into a single "done" checkmark.
 */
export function LifecycleStrip({
  incident,
  plan,
  executions,
}: {
  incident: Incident
  plan: RemediationPlan | null
  executions: ExecutionRecord[]
}) {
  const correlated = Boolean(incident.correlationGroupId)
  const rca = incident.rootCause ?? null

  const latestExecution =
    executions.length > 0
      ? [...executions].sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime())[0]
      : null

  return (
    <div className="flex flex-wrap items-center gap-x-1 gap-y-2 border-b border-border-subtle bg-surface-1 px-4 py-2.5 text-2xs">
      <Step done label="Detected" />
      <Arrow />
      <Step done={correlated} label="Correlated" />
      <Arrow />
      <Step done={rca !== null} label="RCA" sublabel={rca ? rca.confidence : undefined} />

      <Branch />

      <div className="flex flex-wrap items-center gap-3 rounded-sm border border-border-subtle bg-surface-2 px-2.5 py-1.5">
        <Cluster label="Plan">
          {plan ? <StatusBadge status={plan.status} /> : <Muted>None</Muted>}
        </Cluster>
        <ClusterDivider />
        <Cluster label="Execution">
          {latestExecution ? <StatusBadge status={latestExecution.executionStatus} /> : <Muted>None</Muted>}
        </Cluster>
        <ClusterDivider />
        <Cluster label="Verification">
          {latestExecution ? (
            <StatusBadge status={latestExecution.verificationStatus} />
          ) : (
            <Muted>N/A</Muted>
          )}
        </Cluster>
      </div>
    </div>
  )
}

function Step({ done, label, sublabel }: { done: boolean; label: string; sublabel?: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <span
        className={clsx(
          'flex h-4 w-4 items-center justify-center rounded-full text-[9px]',
          done ? 'bg-status-healthy text-surface-0' : 'border border-border-strong text-text-disabled',
        )}
      >
        {done ? '✓' : ''}
      </span>
      <span className={done ? 'text-text-primary' : 'text-text-muted'}>{label}</span>
      {sublabel ? <span className="text-text-muted">({sublabel})</span> : null}
    </div>
  )
}

function Arrow() {
  return <span className="px-0.5 text-text-disabled">→</span>
}

function Branch() {
  return <span className="px-1 text-text-disabled" aria-hidden="true">⌐</span>
}

function Cluster({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-1.5">
      <span className="text-text-muted">{label}</span>
      {children}
    </div>
  )
}

function ClusterDivider() {
  return <span className="text-border-strong">|</span>
}

function Muted({ children }: { children: React.ReactNode }) {
  return <span className="text-text-disabled">{children}</span>
}
