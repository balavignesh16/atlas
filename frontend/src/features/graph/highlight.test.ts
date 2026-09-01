import { describe, expect, it } from 'vitest'
import type { GraphSnapshot, Incident } from '@/api/types'
import { computeHighlight } from './highlight'

function makeIncident(overrides: Partial<Incident>): Incident {
  return {
    incidentId: 'inc-1',
    status: 'OPEN',
    severity: 'CRITICAL',
    title: 'test incident',
    description: '',
    startedAt: '2026-01-01T00:00:00Z',
    lastUpdatedAt: '2026-01-01T00:00:00Z',
    fingerprint: 'fp',
    rootService: 'atlas-payment-service',
    rootOperation: 'op',
    affectedServices: [],
    affectedOperations: [],
    affectedEdges: [],
    traceCount: 0,
    failureCount: 0,
    traceIds: [],
    evidenceIds: [],
    detectionReason: '',
    ...overrides,
  }
}

const snapshot: GraphSnapshot = {
  nodes: ['atlas-gateway', 'atlas-order-service', 'atlas-payment-service', 'atlas-inventory-service'],
  edges: [
    { source: 'atlas-gateway', target: 'atlas-order-service', call_count: 10, first_observed: '', last_observed: '', average_duration_ms: 5, error_count: 0, status_counts: {} },
    { source: 'atlas-order-service', target: 'atlas-payment-service', call_count: 10, first_observed: '', last_observed: '', average_duration_ms: 5, error_count: 8, status_counts: {} },
    { source: 'atlas-order-service', target: 'atlas-inventory-service', call_count: 10, first_observed: '', last_observed: '', average_duration_ms: 5, error_count: 0, status_counts: {} },
  ],
}

describe('computeHighlight', () => {
  it('is inactive with no incident', () => {
    const result = computeHighlight(snapshot, undefined)
    expect(result.active).toBe(false)
  })

  it('is inactive with no snapshot', () => {
    const result = computeHighlight(undefined, makeIncident({ affectedServices: ['atlas-payment-service'] }))
    expect(result.active).toBe(false)
  })

  it('marks the literal affectedServices as affected', () => {
    const incident = makeIncident({ affectedServices: ['atlas-payment-service'] })
    const result = computeHighlight(snapshot, incident)
    expect(result.nodes.get('atlas-payment-service')).toBe('affected')
  })

  it('marks a literal affectedEdges entry as affected using the real edgeKey format', () => {
    const incident = makeIncident({
      affectedServices: ['atlas-order-service', 'atlas-payment-service'],
      affectedEdges: ['atlas-order-service->atlas-payment-service'],
    })
    const result = computeHighlight(snapshot, incident)
    expect(result.edges.get('atlas-order-service->atlas-payment-service')).toBe('affected')
  })

  it('marks a node adjacent to an affected service as related, not affected', () => {
    const incident = makeIncident({ affectedServices: ['atlas-order-service'] })
    const result = computeHighlight(snapshot, incident)
    expect(result.nodes.get('atlas-order-service')).toBe('affected')
    expect(result.nodes.get('atlas-payment-service')).toBe('related')
    expect(result.nodes.get('atlas-inventory-service')).toBe('related')
  })

  it('marks a node with no path to any affected service as unrelated', () => {
    const incident = makeIncident({ affectedServices: ['atlas-payment-service'] })
    const result = computeHighlight(snapshot, incident)
    expect(result.nodes.get('atlas-gateway')).toBe('unrelated')
  })

  it('never marks a node as affected purely because its name looks related', () => {
    const incident = makeIncident({ affectedServices: ['atlas-payment-service'] })
    const result = computeHighlight(snapshot, incident)
    // atlas-inventory-service is topologically unrelated to payment here
    // and must not be inferred as affected/related from name similarity.
    expect(result.nodes.get('atlas-inventory-service')).toBe('unrelated')
  })
})
