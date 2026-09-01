import { useState } from 'react'
import { useIncidentEvidence } from '@/api/evidence'
import { Panel } from '@/components/ui/Panel'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { Skeleton } from '@/components/ui/Skeleton'
import { Timestamp } from '@/components/ui/Timestamp'
import { IdChip } from '@/components/ui/IdChip'
import type { Evidence } from '@/api/types'

/**
 * Evidence, sorted client-side by real `timestamp` (see api/evidence.ts for
 * why: the backend's dedicated /timeline endpoint is unsorted and redundant
 * with /evidence, verified against source). Each row is collapsed by
 * default; expanding shows the specific typed fields on the Evidence model,
 * never a raw JSON dump.
 */
export function EvidenceTimeline({ incidentId }: { incidentId: string }) {
  const { data, isPending, isError, error, refetch } = useIncidentEvidence(incidentId)

  if (isPending) {
    return (
      <Panel title="Evidence Timeline">
        <div className="space-y-2 p-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      </Panel>
    )
  }

  if (isError) {
    return (
      <Panel title="Evidence Timeline">
        <ErrorState error={error} onRetry={() => refetch()} />
      </Panel>
    )
  }

  if (data.length === 0) {
    return (
      <Panel title="Evidence Timeline">
        <EmptyState title="No evidence recorded" description="No evidence items have been attached to this incident." />
      </Panel>
    )
  }

  const sorted = [...data].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())

  return (
    <Panel title={`Evidence Timeline (${sorted.length})`}>
      <ul className="divide-y divide-border-subtle">
        {sorted.map((item) => (
          <EvidenceRow key={item.evidenceId} evidence={item} />
        ))}
      </ul>
    </Panel>
  )
}

function EvidenceRow({ evidence }: { evidence: Evidence }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <li>
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
        className="flex w-full items-center gap-3 px-3 py-2 text-left text-xs hover:bg-surface-2"
      >
        <span className="w-3 shrink-0 text-text-disabled">{expanded ? '▾' : '▸'}</span>
        <Timestamp value={evidence.timestamp} className="w-28 shrink-0 font-mono text-2xs text-text-muted" />
        <span className="w-32 shrink-0 truncate text-text-secondary">{evidence.type.replace(/_/g, ' ')}</span>
        <span className="flex-1 truncate text-text-primary">{evidence.description}</span>
      </button>

      {expanded ? (
        <div className="grid grid-cols-2 gap-x-4 gap-y-1 border-t border-border-subtle bg-surface-2 px-3 py-2.5 pl-9 text-2xs sm:grid-cols-3">
          <Field label="Service" value={evidence.service} />
          <Field label="Operation" value={evidence.operation} />
          <Field label="Source" value={evidence.source} />
          <Field label="Observed" value={String(evidence.observed)} />
          <Field label="Expected" value={String(evidence.expected)} />
          <Field label="Value" value={String(evidence.value)} />
          {evidence.traceId ? <Field label="Trace" value={<IdChip value={evidence.traceId} truncate={6} />} /> : null}
          {evidence.spanId ? <Field label="Span" value={<IdChip value={evidence.spanId} truncate={6} />} /> : null}
        </div>
      ) : null}
    </li>
  )
}

function Field({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col">
      <span className="text-text-disabled">{label}</span>
      <span className="font-mono text-text-secondary">{value}</span>
    </div>
  )
}
