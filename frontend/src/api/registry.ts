import { useQuery } from '@tanstack/react-query'
import { apiFetch, ApiError } from './client'
import type { Service, ServiceIntelligence, ServiceProvenance, ServiceStatus } from './types'

export interface ServicesFilter {
  status?: ServiceStatus
  source?: ServiceProvenance
  q?: string
}

function buildQuery(filter: ServicesFilter): string {
  const params = new URLSearchParams()
  if (filter.status) params.set('status', filter.status)
  if (filter.source) params.set('source', filter.source)
  if (filter.q) params.set('q', filter.q)
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

/**
 * GET /api/v1/services -- the canonical service registry, NOT the live
 * telemetry graph (useGraphSnapshot in api/graph.ts). A service can appear
 * here long after its graph node has expired under DependencyGraph's own,
 * unrelated 300s retention -- that is the entire point of the registry.
 * Filtering happens server-side (Phase 7C's status/source/q query params)
 * so the query key includes the filter, keeping each filter combination
 * its own cache entry. Polled at a low frequency: registry status changes
 * on the order of minutes (ATLAS_REGISTRY_STALE_AFTER_SECONDS defaults to
 * 30 minutes), not seconds.
 */
export function useServices(filter: ServicesFilter = {}) {
  return useQuery({
    queryKey: ['registry', 'services', filter],
    queryFn: () => apiFetch<Service[]>(`/api/v1/services${buildQuery(filter)}`),
    refetchInterval: 30000,
  })
}

/** GET /api/v1/services/{name}. A 404 means this exact name has never been
 * observed by real telemetry -- never treated as an error page. */
export function useService(name: string | null) {
  return useQuery({
    queryKey: ['registry', 'services', name],
    queryFn: () => apiFetch<Service>(`/api/v1/services/${encodeURIComponent(name ?? '')}`),
    enabled: name !== null,
    retry: false,
  })
}

export function isServiceNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

/**
 * GET /api/v1/services/{name}/intelligence -- Phase 7D's composed,
 * read-only per-service view (registry + live dependency graph + incident
 * history), assembled fresh on every request from three independent
 * sources. A 404 here means the name is unknown to all three at once;
 * partial evidence (e.g. graph-only, no registry entry yet) is still a
 * real 200 -- see docs/registry.md's "Service Intelligence" section.
 */
export function useServiceIntelligence(name: string | null) {
  return useQuery({
    queryKey: ['registry', 'services', name, 'intelligence'],
    queryFn: () => apiFetch<ServiceIntelligence>(`/api/v1/services/${encodeURIComponent(name ?? '')}/intelligence`),
    enabled: name !== null,
    retry: false,
  })
}
