import { isNotFound, useGenerateAnalysis, useIncidentAnalysis } from '@/api/analysis'
import { useIdentity } from '@/api/auth'
import { Panel } from '@/components/ui/Panel'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { Skeleton } from '@/components/ui/Skeleton'
import { Button } from '@/components/ui/Button'
import { MutationError } from '@/components/ui/MutationError'
import { PermissionHint } from '@/components/ui/PermissionHint'
import { StatusBadge } from '@/components/status/StatusBadge'
import { canUse, permissionHint } from '@/lib/permissions'

/**
 * GET /incidents/{id}/analysis for the real, already-generated result; a
 * "Generate Analysis" control (below) triggers POST /analyze for real when
 * none exists yet -- see api/analysis.ts for why this is now safe to expose
 * (Module 4 fixed the routing/auth defect that made this unsafe in Phase 2).
 * A 404 on the GET means "no analysis has been generated," a normal and
 * expected state, not an error.
 */
export function AnalysisSection({ incidentId }: { incidentId: string }) {
  const { data, isPending, isError, error, refetch } = useIncidentAnalysis(incidentId)
  const { data: identity } = useIdentity()
  const generateAnalysis = useGenerateAnalysis(incidentId)
  const canGenerate = canUse(identity, 'VIEW')

  if (isPending) {
    return (
      <Panel title="AI Analysis">
        <div className="space-y-2 p-3">
          <Skeleton className="h-3.5 w-full" />
          <Skeleton className="h-3.5 w-5/6" />
          <Skeleton className="h-3.5 w-2/3" />
        </div>
      </Panel>
    )
  }

  if (isError) {
    if (isNotFound(error)) {
      return (
        <Panel
          title="AI Analysis"
          action={
            <Button
              size="sm"
              variant="secondary"
              onClick={() => generateAnalysis.mutate()}
              disabled={generateAnalysis.isPending || !canGenerate}
            >
              {generateAnalysis.isPending ? 'Generating…' : 'Generate Analysis'}
            </Button>
          }
        >
          <div className="p-3">
            <EmptyState
              title="No analysis generated"
              description="AI analysis has not been generated for this incident."
            />
            {!canGenerate || generateAnalysis.isError ? (
              <div className="px-3 pb-3">
                <PermissionHint hint={permissionHint(identity, 'VIEW')} />
                {generateAnalysis.isError ? <MutationError error={generateAnalysis.error} /> : null}
              </div>
            ) : null}
          </div>
        </Panel>
      )
    }
    return (
      <Panel title="AI Analysis">
        <ErrorState error={error} onRetry={() => refetch()} />
      </Panel>
    )
  }

  return (
    <Panel title="AI Analysis">
      <div className="space-y-3 p-3 text-xs">
        <p className="text-text-primary">{data.executiveSummary}</p>

        <div className="flex items-center gap-2">
          <span className="text-2xs uppercase tracking-wide text-text-muted">Likely root cause</span>
          <StatusBadge status={data.rootCauseConfidence} label={data.rootCauseConfidence} />
        </div>
        <p className="text-text-secondary">{data.likelyRootCause}</p>

        <ListBlock label="Observed facts" items={data.observedFacts.map((f) => f.claim)} />
        <ListBlock label="Inferences" items={data.inferences.map((f) => f.claim)} muted />
        <ListBlock label="Alternative explanations" items={data.alternativeExplanations} muted />
        <ListBlock label="Missing evidence" items={data.missingEvidence} muted />

        {data.limitations ? (
          <p className="border-t border-border-subtle pt-2 text-2xs text-text-muted">{data.limitations}</p>
        ) : null}

        <p className="text-2xs text-text-disabled">
          {data.provider}/{data.model}
        </p>
      </div>
    </Panel>
  )
}

function ListBlock({ label, items, muted }: { label: string; items: string[]; muted?: boolean }) {
  if (items.length === 0) return null
  return (
    <div>
      <p className="text-2xs uppercase tracking-wide text-text-muted">{label}</p>
      <ul className={`mt-1 list-inside list-disc space-y-0.5 ${muted ? 'text-text-muted' : 'text-text-secondary'}`}>
        {items.map((item, i) => (
          <li key={i}>{item}</li>
        ))}
      </ul>
    </div>
  )
}
