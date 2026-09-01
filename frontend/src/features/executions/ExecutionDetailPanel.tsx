import { Link } from '@tanstack/react-router'
import { useExecution } from '@/api/executions'
import { useRemediationPlanById } from '@/api/remediation'
import { StatusBadge } from '@/components/status/StatusBadge'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { IdChip } from '@/components/ui/IdChip'
import { SidePanel } from '@/components/ui/SidePanel'
import { Skeleton } from '@/components/ui/Skeleton'
import { Timestamp } from '@/components/ui/Timestamp'

/**
 * SAFETY-CRITICAL: execution and verification are rendered as two entirely
 * separate rows with their own badges, never merged into one "result" --
 * see ExecutionPanel.tsx (Incident Detail) for the same rule applied there.
 * Every field here comes directly from the real ExecutionRecord/
 * RemediationPlan returned by GET /executions/{id} and
 * GET /remediation/{planId}; absent fields (approver, message, error) are
 * omitted rather than shown as "N/A" or inferred.
 */
export function ExecutionDetailPanel({ executionId, onClose }: { executionId: string | null; onClose: () => void }) {
  const { data: record, isPending, isError, error } = useExecution(executionId)
  const { data: plan } = useRemediationPlanById(record?.planId ?? null)

  return (
    <SidePanel
      open={executionId !== null}
      onOpenChange={(open) => !open && onClose()}
      title={record ? record.action : 'Execution'}
      subtitle={record ? record.service : undefined}
    >
      {isPending ? (
        <div className="space-y-2 p-4">
          <Skeleton className="h-3.5 w-full" />
          <Skeleton className="h-3.5 w-5/6" />
          <Skeleton className="h-3.5 w-2/3" />
        </div>
      ) : isError ? (
        <div className="p-4">
          <ErrorState error={error} />
        </div>
      ) : (
        <div className="space-y-4 p-4 text-xs">
          <div className="flex items-center justify-between gap-2">
            <IdChip value={record.executionId} truncate={8} />
            <Link to="/incidents/$incidentId" params={{ incidentId: record.incidentId }}>
              <Button variant="secondary" size="sm">
                Originating incident
              </Button>
            </Link>
          </div>

          <div className="grid grid-cols-2 gap-3 border-y border-border-subtle py-3">
            <StatusBlock label="Execution status" status={record.executionStatus} />
            <StatusBlock label="Verification status" status={record.verificationStatus} />
          </div>

          <div className="space-y-1">
            <Row label="Service" value={record.service} />
            <Row label="Action" value={<span className="font-mono">{record.action}</span>} />
            <Row label="Started" value={<Timestamp value={record.startedAt} />} />
            {record.finishedAt ? <Row label="Finished" value={<Timestamp value={record.finishedAt} />} /> : null}
            {record.approver ? <Row label="Executed by" value={record.approver} /> : null}
            {record.approvalFingerprint ? (
              <Row label="Approval fingerprint" value={<IdChip value={record.approvalFingerprint} truncate={6} />} />
            ) : null}
          </div>

          {record.message ? (
            <div className="rounded-sm border border-border-subtle bg-surface-2 p-2 text-text-secondary">{record.message}</div>
          ) : null}
          {record.error ? (
            <div className="rounded-sm border border-status-critical/40 bg-status-critical-bg p-2 text-status-critical">
              {record.error}
            </div>
          ) : null}

          <div className="border-t border-border-subtle pt-3">
            <p className="mb-1.5 text-2xs uppercase tracking-wide text-text-muted">Remediation plan</p>
            {plan ? (
              <div className="space-y-1.5 rounded-sm border border-border-subtle bg-surface-2 p-2.5">
                <div className="flex items-center justify-between gap-2">
                  <StatusBadge status={plan.status} />
                  <StatusBadge status={plan.riskLevel} label={`${plan.riskLevel} RISK`} />
                </div>
                <p className="text-text-secondary">{plan.rationale}</p>
                <Link
                  to="/incidents/$incidentId"
                  params={{ incidentId: record.incidentId }}
                  className="inline-block text-2xs text-accent hover:underline"
                >
                  View full plan on incident →
                </Link>
              </div>
            ) : (
              <EmptyState title="Plan not available" description="The referenced plan could not be loaded." />
            )}
          </div>

          <Link to="/graph" search={{ highlight: record.incidentId }}>
            <Button variant="secondary" size="sm">
              View affected services in graph
            </Button>
          </Link>
        </div>
      )}
    </SidePanel>
  )
}

function StatusBlock({ label, status }: { label: string; status: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-2xs uppercase tracking-wide text-text-muted">{label}</span>
      <StatusBadge status={status} />
    </div>
  )
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-0.5">
      <span className="text-text-muted">{label}</span>
      <span className="text-right text-text-primary">{value}</span>
    </div>
  )
}
