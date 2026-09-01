import type { DependencyEdge } from '@/api/types'
import type { HighlightState } from './highlight'

/**
 * error_count/call_count is a real backend-provided pair; the RATIO is a
 * presentational derivation, not a backend metric. Never label this as a
 * "health score" or anything the backend doesn't actually claim. Returns
 * null when call_count is 0 (nothing to divide -- not zero, not unknown).
 */
export function deriveErrorRate(edge: Pick<DependencyEdge, 'call_count' | 'error_count'>): number | null {
  if (edge.call_count <= 0) return null
  return edge.error_count / edge.call_count
}

export function formatErrorRate(rate: number | null): string {
  if (rate === null) return '—'
  return `${(rate * 100).toFixed(1)}%`
}

/** The exact key format the backend itself uses internally (graph.go's
 * edgeKey) and the format Incident.affectedEdges strings are already
 * written in -- reusing it means blast-radius matching is a real string
 * comparison, never a name-based guess. */
export function edgeKey(source: string, target: string): string {
  return `${source}->${target}`
}

const MIN_EDGE_WIDTH = 1.25
const MAX_EDGE_WIDTH = 4

/** Stroke width scales with real call_count (log-scaled so one hot edge
 * doesn't swamp the rest); color/emphasis reflects real error_count, never
 * a fabricated health score. */
export function widthForCallCount(callCount: number): number {
  if (callCount <= 0) return MIN_EDGE_WIDTH
  const scaled = MIN_EDGE_WIDTH + Math.log10(callCount + 1) * 0.9
  return Math.min(MAX_EDGE_WIDTH, scaled)
}

export function colorVarFor(highlight: HighlightState, hasErrors: boolean, selected: boolean): string {
  if (selected) return 'var(--color-accent)'
  if (highlight === 'affected') return 'var(--color-status-critical)'
  if (highlight === 'related') return 'var(--color-status-warning)'
  if (hasErrors) return 'var(--color-status-failed)'
  return 'var(--color-border-strong)'
}
