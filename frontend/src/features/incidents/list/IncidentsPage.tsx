import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'
import { incidentsRoute } from '@/app/routes'
import { useAllIncidents } from '@/api/incidents'
import { EmptyState } from '@/components/ui/EmptyState'
import { ErrorState } from '@/components/ui/ErrorState'
import { TableRowSkeleton } from '@/components/ui/Skeleton'
import { IncidentTable } from '../IncidentTable'
import { sortIncidents } from '../sort'

/**
 * This page intentionally does NOT fetch per-incident execution state the
 * way the Command Center's open-incidents table does -- that overlay is
 * bounded there to "currently open incidents" (a small set). Applied to
 * every incident in the full, unbounded incident history here, the same
 * N+1 fetch pattern would not scale and there is no pagination on
 * GET /api/v1/incidents to bound it. This page shows each incident's own
 * `status` field only. See the Phase 1 report for this trade-off.
 */
export function IncidentsPage() {
  const search = incidentsRoute.useSearch()
  const navigate = useNavigate({ from: '/incidents' })
  const { data: incidents, isPending, isError, error, refetch } = useAllIncidents()

  const filtered = useMemo(() => {
    if (!incidents) return []
    return incidents.filter((incident) => {
      if (search.status && incident.status !== search.status) return false
      if (search.severity && incident.severity !== search.severity) return false
      if (search.service && incident.rootService !== search.service) return false
      return true
    })
  }, [incidents, search])

  const sorted = sortIncidents(filtered)
  const services = useMemo(
    () => Array.from(new Set((incidents ?? []).map((i) => i.rootService))).sort(),
    [incidents],
  )

  function setFilter<K extends 'status' | 'severity' | 'service'>(key: K, value: string) {
    navigate({
      search: (prev) => ({ ...prev, [key]: value === '' ? undefined : value }),
    })
  }

  if (isError) {
    return <ErrorState error={error} onRetry={() => refetch()} />
  }

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b border-border-subtle bg-surface-1 px-4 py-2.5">
        <FilterSelect
          label="Status"
          value={search.status ?? ''}
          onChange={(v) => setFilter('status', v)}
          options={['OPEN', 'ACKNOWLEDGED', 'RESOLVED']}
        />
        <FilterSelect
          label="Severity"
          value={search.severity ?? ''}
          onChange={(v) => setFilter('severity', v)}
          options={['CRITICAL', 'WARNING', 'INFO']}
        />
        <FilterSelect
          label="Service"
          value={search.service ?? ''}
          onChange={(v) => setFilter('service', v)}
          options={services}
        />
        <span className="ml-auto text-2xs text-text-muted">
          {isPending ? '—' : `${sorted.length} of ${incidents?.length ?? 0}`}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        {isPending ? (
          <table className="w-full">
            <tbody>
              <TableRowSkeleton columns={5} />
              <TableRowSkeleton columns={5} />
              <TableRowSkeleton columns={5} />
              <TableRowSkeleton columns={5} />
            </tbody>
          </table>
        ) : sorted.length === 0 ? (
          <EmptyState
            title="No matching incidents"
            description={incidents && incidents.length > 0 ? 'Try clearing a filter.' : 'Atlas has no recorded incidents.'}
          />
        ) : (
          <IncidentTable incidents={sorted} />
        )}
      </div>
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
