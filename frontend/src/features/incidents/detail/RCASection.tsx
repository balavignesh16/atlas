import { useIncidentRCA } from '@/api/rca'
import { ApiError } from '@/api/client'
import { Panel } from '@/components/ui/Panel'
import { EmptyState } from '@/components/ui/EmptyState'
import { Skeleton } from '@/components/ui/Skeleton'
import { IdChip } from '@/components/ui/IdChip'
import { StatusBadge } from '@/components/status/StatusBadge'

/**
 * GET /incidents/{id}/rca. Confidence is shown exactly as the backend
 * reports it (a string like LOW/MEDIUM/HIGH/AMBIGUOUS) plus the raw score --
 * never converted into an invented percentage or a fabricated precision the
 * backend never claimed.
 */
export function RCASection({ incidentId }: { incidentId: string }) {
  const { data, isPending, isError, error } = useIncidentRCA(incidentId)

  if (isPending) {
    return (
      <Panel title="Root Cause Analysis">
        <div className="space-y-2 p-3">
          <Skeleton className="h-3.5 w-2/3" />
          <Skeleton className="h-3.5 w-1/2" />
          <Skeleton className="h-3.5 w-full" />
        </div>
      </Panel>
    )
  }

  if (isError) {
    if (error instanceof ApiError && error.status === 404) {
      return (
        <Panel title="Root Cause Analysis">
          <EmptyState title="No RCA verdict" description="Correlation has not produced a root-cause verdict for this incident yet." />
        </Panel>
      )
    }
    return (
      <Panel title="Root Cause Analysis">
        <EmptyState title="RCA unavailable" description={error instanceof Error ? error.message : 'Could not load root cause analysis.'} />
      </Panel>
    )
  }

  return (
    <Panel title="Root Cause Analysis">
      <div className="space-y-3 p-3 text-xs">
        <div className="flex items-baseline justify-between gap-4">
          <span className="text-text-primary">{data.rootCause}</span>
          <span className="flex items-center gap-2 shrink-0">
            <StatusBadge status={data.confidence} label={data.confidence} />
            <span className="text-2xs text-text-muted">score {data.score.toFixed(2)}</span>
          </span>
        </div>

        {data.candidates.length > 1 ? (
          <div>
            <p className="text-2xs uppercase tracking-wide text-text-muted">Other candidates considered</p>
            <ul className="mt-1 space-y-0.5 text-text-secondary">
              {data.candidates
                .filter((c) => c !== data.rootCause)
                .map((c) => (
                  <li key={c}>{c}</li>
                ))}
            </ul>
          </div>
        ) : null}

        {data.reasoning.length > 0 ? (
          <div>
            <p className="text-2xs uppercase tracking-wide text-text-muted">Reasoning</p>
            <ul className="mt-1 list-inside list-disc space-y-0.5 text-text-secondary">
              {data.reasoning.map((r, i) => (
                <li key={i}>{r}</li>
              ))}
            </ul>
          </div>
        ) : null}

        {data.evidenceIds.length > 0 ? (
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="text-2xs uppercase tracking-wide text-text-muted">Evidence</span>
            {data.evidenceIds.map((id) => (
              <IdChip key={id} value={id} truncate={6} />
            ))}
          </div>
        ) : null}

        {data.limitations.length > 0 ? (
          <p className="border-t border-border-subtle pt-2 text-2xs text-text-muted">{data.limitations.join(' ')}</p>
        ) : null}
      </div>
    </Panel>
  )
}
