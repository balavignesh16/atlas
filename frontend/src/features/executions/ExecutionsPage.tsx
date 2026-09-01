import { useNavigate, useParams } from '@tanstack/react-router'
import { executionsRoute } from '@/app/routes'
import { useRecentExecutions } from '@/api/executions'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { TableRowSkeleton } from '@/components/ui/Skeleton'
import { ExecutionDetailPanel } from './ExecutionDetailPanel'
import { ExecutionTable } from './ExecutionTable'
import { filterExecutions } from './filter'

/**
 * There is no global execution log endpoint (see api/executions.ts). This
 * page is honest about that: it shows executions composed from a bounded
 * set of the most recently-started incidents, and says so in the header
 * rather than presenting itself as a complete history.
 */
export function ExecutionsPage() {
  const search = executionsRoute.useSearch()
  const navigate = useNavigate({ from: '/executions' })
  const { executionId } = useParams({ strict: false })

  const { records, isPending, isError, error, forbidden, consideredIncidentCount, totalIncidentCount, limit } =
    useRecentExecutions()

  const filtered = filterExecutions(records, search.q ?? '')

  function setQuery(value: string) {
    navigate({ search: (prev) => ({ ...prev, q: value === '' ? undefined : value }) })
  }

  function closeDetail() {
    navigate({ to: '/executions', search: (prev) => prev })
  }

  if (isError) {
    return <ErrorState error={error} />
  }

  if (forbidden && records.length === 0 && !isPending) {
    return (
      <EmptyState
        title="Execution records unavailable"
        description={
          <>
            Viewing execution history requires <span className="font-mono">READ_AUDIT</span>.
          </>
        }
      />
    )
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-3 border-b border-border-subtle bg-surface-1 px-4 py-2.5">
        <input
          type="text"
          value={search.q ?? ''}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search by ID, service, or action…"
          aria-label="Search executions"
          className="h-7 w-72 rounded-sm border border-border-default bg-surface-2 px-2 text-xs text-text-primary outline-none placeholder:text-text-disabled focus-visible:border-accent"
        />
        <span className="ml-auto text-2xs text-text-muted">
          {isPending
            ? '—'
            : `${filtered.length} shown · composed from the ${consideredIncidentCount} most recent incidents of ${totalIncidentCount} total (limit ${limit})`}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isPending ? (
          <table className="w-full">
            <tbody>
              <TableRowSkeleton columns={7} />
              <TableRowSkeleton columns={7} />
              <TableRowSkeleton columns={7} />
            </tbody>
          </table>
        ) : filtered.length === 0 ? (
          <EmptyState
            title="No executions found"
            description={
              records.length > 0
                ? 'Try clearing the search.'
                : 'No actions have been executed for any recently-tracked incident.'
            }
          />
        ) : (
          <ExecutionTable records={filtered} />
        )}
      </div>

      <ExecutionDetailPanel executionId={executionId ?? null} onClose={closeDetail} />
    </div>
  )
}
