import { useEffect, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { incidentDetailRoute } from '@/app/routes'
import { ApiError } from '@/api/client'
import { useIncident, useIncidentExecutions } from '@/api/incidents'
import { useRemediationPlan } from '@/api/remediation'
import type { ExecutionRecord, Incident } from '@/api/types'
import { SeverityIndicator } from '@/components/status/SeverityIndicator'
import { StatusBadge } from '@/components/status/StatusBadge'
import { ErrorState } from '@/components/ui/ErrorState'
import { EmptyState } from '@/components/ui/EmptyState'
import { IdChip } from '@/components/ui/IdChip'
import { Timestamp } from '@/components/ui/Timestamp'
import { PollIndicator } from '@/components/layout/PollIndicator'
import { AnalysisSection } from './AnalysisSection'
import { BlastRadiusSection } from './BlastRadiusSection'
import { DetailSkeleton } from './DetailSkeleton'
import { EvidenceTimeline } from './EvidenceTimeline'
import { ExecutionPanel } from './ExecutionPanel'
import { LifecycleStrip } from './LifecycleStrip'
import { RCASection } from './RCASection'
import { RemediationPanel } from './RemediationPanel'

/**
 * Polling cadence, computed from real fetched state only (never a fixed
 * timer that runs regardless of what's happening):
 *  - 2s while any execution is VERIFYING (the fastest-changing real state)
 *  - 5s while the incident is open/acknowledged, or an execution is still
 *    EXECUTING/PRECONDITION_CHECK
 *  - stopped once the incident is RESOLVED and nothing is still executing
 *
 * This synchronizes an external system (the query observers' refetch
 * schedule) with newly-arrived data, which is exactly what an effect is
 * for -- deriving it during render would mean the very first render after
 * new data arrives uses last cycle's stale interval, and mutating a ref
 * during render to work around that is its own, worse anti-pattern (it
 * makes the read during render unstable under concurrent rendering).
 */
function computePollMs(incident: Incident | undefined, executions: ExecutionRecord[] | undefined): number | false {
  if (!incident) return 5000
  if (executions?.some((e) => e.verificationStatus === 'VERIFYING')) return 2000
  const executing = executions?.some((e) => e.executionStatus === 'EXECUTING' || e.executionStatus === 'PRECONDITION_CHECK')
  if (incident.status === 'RESOLVED' && !executing) return false
  return 5000
}

export function IncidentDetailPage() {
  const { incidentId } = incidentDetailRoute.useParams()
  const [pollMs, setPollMs] = useState<number | false>(5000)

  const { data: incident, isPending, isError, error, refetch, dataUpdatedAt } = useIncident(incidentId, pollMs)
  const { data: executions } = useIncidentExecutions(incidentId, pollMs)
  const { data: plan } = useRemediationPlan(incidentId)

  useEffect(() => {
    const next = computePollMs(incident, executions)
    setPollMs((current) => (current === next ? current : next))
  }, [incident, executions])

  if (isError) {
    if (error instanceof ApiError && error.status === 404) {
      return (
        <div className="p-4">
          <EmptyState title="Incident not found" description={`No incident exists with ID ${incidentId}.`} />
        </div>
      )
    }
    return <ErrorState error={error} onRetry={() => refetch()} />
  }

  if (isPending) {
    return <DetailSkeleton />
  }

  return (
    <div className="space-y-3 pb-6">
      <div className="flex items-start justify-between gap-4 p-4">
        <div>
          <Link to="/incidents" className="text-2xs text-text-muted hover:text-text-secondary">
            ← Incidents
          </Link>
          <div className="mt-1 flex items-center gap-2">
            <SeverityIndicator severity={incident.severity} />
            <h1 className="text-lg font-semibold text-text-primary">{incident.title}</h1>
          </div>
          <p className="mt-1 text-xs text-text-secondary">{incident.description}</p>
          <div className="mt-2 flex flex-wrap items-center gap-3 text-2xs text-text-muted">
            <StatusBadge status={incident.status} />
            <span>
              Detected <Timestamp value={incident.startedAt} />
            </span>
            <span>
              Updated <Timestamp value={incident.lastUpdatedAt} />
            </span>
            {incident.resolvedAt ? (
              <span>
                Resolved <Timestamp value={incident.resolvedAt} />
              </span>
            ) : null}
            <IdChip value={incident.incidentId} />
            <IdChip value={incident.fingerprint} truncate={6} />
          </div>
        </div>
        <PollIndicator polling={pollMs !== false} lastUpdatedAt={dataUpdatedAt} />
      </div>

      <LifecycleStrip incident={incident} plan={plan ?? null} executions={executions ?? []} />

      <div className="grid grid-cols-1 gap-3 px-4 lg:grid-cols-[3fr_2fr]">
        <div className="space-y-3">
          <RCASection incidentId={incidentId} />
          <RemediationPanel incidentId={incidentId} />
          <ExecutionPanel incidentId={incidentId} plan={plan ?? null} />
        </div>
        <div className="space-y-3">
          <BlastRadiusSection incident={incident} />
          <AnalysisSection incidentId={incidentId} />
        </div>
      </div>

      <div className="px-4">
        <EvidenceTimeline incidentId={incidentId} />
      </div>
    </div>
  )
}
