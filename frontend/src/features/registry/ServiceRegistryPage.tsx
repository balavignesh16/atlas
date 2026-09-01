import { useNavigate, useParams } from '@tanstack/react-router'
import { servicesRoute } from '@/app/routes'
import { useServices } from '@/api/registry'
import type { ServiceProvenance, ServiceStatus } from '@/api/types'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { TableRowSkeleton } from '@/components/ui/Skeleton'
import { ServiceDetailPanel } from './ServiceDetailPanel'
import { ServiceTable } from './ServiceTable'
import { sortServices } from './sort'

const STATUS_OPTIONS: ServiceStatus[] = ['ACTIVE', 'STALE', 'RETIRED']
const SOURCE_OPTIONS: ServiceProvenance[] = ['OBSERVED_TELEMETRY', 'DECLARED', 'DOCKER', 'KUBERNETES', 'CONFIG', 'INFERRED']

/**
 * The canonical service registry -- distinct from the live telemetry graph
 * at /graph. A service can be listed here as STALE or RETIRED long after
 * its graph node has already expired; that is the entire reason this page
 * exists rather than just linking to the graph. Filters (status/source/q)
 * are URL-driven and resolved server-side (Phase 7C), matching the
 * pattern IncidentsPage/ExecutionsPage already established.
 */
export function ServiceRegistryPage() {
  const search = servicesRoute.useSearch()
  const { serviceName } = useParams({ strict: false })
  const navigate = useNavigate({ from: '/services' })
  const { data: services, isPending, isError, error, refetch } = useServices(search)

  if (isError) {
    return <ErrorState error={error} onRetry={() => refetch()} />
  }

  const sorted = services ? sortServices(services) : []

  function setFilter<K extends keyof typeof search>(key: K, value: string) {
    navigate({ search: (prev) => ({ ...prev, [key]: value === '' ? undefined : value }) })
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-border-subtle bg-surface-1 px-4 py-2.5">
        <h1 className="text-xs font-semibold uppercase tracking-wide text-text-secondary">Service Registry</h1>
        <input
          type="text"
          value={search.q ?? ''}
          onChange={(e) => setFilter('q', e.target.value)}
          placeholder="Search by name…"
          aria-label="Search services"
          className="h-7 w-48 rounded-sm border border-border-default bg-surface-2 px-2 text-xs text-text-primary outline-none placeholder:text-text-disabled focus-visible:border-accent"
        />
        <FilterSelect label="Status" value={search.status ?? ''} onChange={(v) => setFilter('status', v)} options={STATUS_OPTIONS} />
        <FilterSelect label="Source" value={search.source ?? ''} onChange={(v) => setFilter('source', v)} options={SOURCE_OPTIONS} />
        <span className="ml-auto text-2xs text-text-muted">
          {isPending ? '—' : `${sorted.length} known service${sorted.length === 1 ? '' : 's'}`}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isPending ? (
          <table className="w-full">
            <tbody>
              <TableRowSkeleton columns={5} />
              <TableRowSkeleton columns={5} />
              <TableRowSkeleton columns={5} />
            </tbody>
          </table>
        ) : sorted.length === 0 ? (
          <EmptyState
            title="No services found"
            description={
              search.status || search.source || search.q
                ? 'Try clearing a filter.'
                : 'Services appear here automatically once Atlas observes real OpenTelemetry traffic naming them.'
            }
          />
        ) : (
          <ServiceTable services={sorted} />
        )}
      </div>

      <ServiceDetailPanel serviceName={serviceName ?? null} onClose={() => navigate({ to: '/services', search: (prev) => prev })} />
    </div>
  )
}

function FilterSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string
  value: string
  onChange: (value: string) => void
  options: string[]
}) {
  return (
    <label className="flex items-center gap-1.5 text-2xs text-text-muted">
      {label}
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded-sm border border-border-default bg-surface-2 px-1.5 py-1 text-2xs text-text-primary outline-none focus-visible:border-accent"
      >
        <option value="">All</option>
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </label>
  )
}
