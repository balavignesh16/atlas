import type { ServiceProvenance, ServiceStatus } from '@/api/types'

export type ServicesSearch = {
  status?: ServiceStatus
  source?: ServiceProvenance
  q?: string
}

const VALID_STATUSES: ServiceStatus[] = ['ACTIVE', 'STALE', 'RETIRED']
const VALID_SOURCES: ServiceProvenance[] = ['OBSERVED_TELEMETRY', 'DECLARED', 'DOCKER', 'KUBERNETES', 'CONFIG', 'INFERRED']

/** Pure so it's unit-testable without a router instance, mirroring the
 * pattern used for incidents/graph/executions search-param parsing. An
 * unrecognized status/source value resolves to "no filter" rather than
 * silently passing a value the backend would reject through to the URL. */
export function parseServicesSearch(search: Record<string, unknown>): ServicesSearch {
  const status = typeof search.status === 'string' && (VALID_STATUSES as string[]).includes(search.status) ? (search.status as ServiceStatus) : undefined
  const source =
    typeof search.source === 'string' && (VALID_SOURCES as string[]).includes(search.source) ? (search.source as ServiceProvenance) : undefined
  const q = typeof search.q === 'string' && search.q.length > 0 ? search.q : undefined

  return { status, source, q }
}
