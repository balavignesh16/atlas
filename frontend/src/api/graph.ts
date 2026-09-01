import { useQuery } from '@tanstack/react-query'
import { apiFetch, ApiError } from './client'
import type { DependencyEdge, GraphSnapshot, ServiceDependencies } from './types'

/** Full dependency graph (nodes + edges). Polled at a low frequency --
 * this topology changes slowly relative to incident state. */
export function useGraphSnapshot() {
  return useQuery({
    queryKey: ['graph', 'snapshot'],
    queryFn: () => apiFetch<GraphSnapshot>('/api/v1/graph'),
    refetchInterval: 20000,
  })
}

export function useGraphEdges() {
  return useQuery({
    queryKey: ['graph', 'edges'],
    queryFn: () => apiFetch<DependencyEdge[] | null>('/api/v1/graph/edges'),
    select: (data) => data ?? [],
    refetchInterval: 20000,
  })
}

/** Fetched on demand (service inspector open), not prefetched for every
 * node -- there is no bulk endpoint and prefetching all services would be
 * an unbounded N+1 for no benefit over fetching the one being inspected. */
export function useServiceDependencies(serviceName: string | null) {
  return useQuery({
    queryKey: ['graph', 'services', serviceName],
    queryFn: () => apiFetch<ServiceDependencies>(`/api/v1/graph/services/${encodeURIComponent(serviceName ?? '')}`),
    enabled: serviceName !== null,
    retry: false,
  })
}

/** A 404 here means "no dependencies observed for this service" -- every
 * node in the graph came from at least one recorded edge, so this is a
 * real, if unlikely, honest empty state rather than a fabricated one. */
export function isServiceNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}
