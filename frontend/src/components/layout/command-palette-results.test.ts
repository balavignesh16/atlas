import { describe, expect, it } from 'vitest'
import type { ExecutionRecord, Incident } from '@/api/types'
import { buildCommandResults } from './command-palette-results'

function makeIncident(id: string, overrides: Partial<Incident> = {}): Incident {
  return {
    incidentId: id,
    status: 'OPEN',
    severity: 'CRITICAL',
    title: `Incident ${id}`,
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

function makeExecution(id: string, overrides: Partial<ExecutionRecord> = {}): ExecutionRecord {
  return {
    executionId: id,
    planId: 'plan-1',
    actionId: 'action-1',
    incidentId: 'inc-1',
    service: 'atlas-order-service',
    action: 'RESTART_SERVICE',
    evidenceIds: [],
    approver: '',
    approvalFingerprint: '',
    startedAt: '2026-01-01T00:00:00Z',
    executionStatus: 'EXECUTED',
    verificationStatus: 'VERIFIED',
    ...overrides,
  }
}

describe('buildCommandResults', () => {
  it('shows all nav commands and no jump/search results for an empty query', () => {
    const results = buildCommandResults('', [makeIncident('inc-1')], [])
    const navResults = results.filter((r) => r.kind === 'nav')
    expect(navResults).toHaveLength(4)
    expect(results.some((r) => r.kind === 'jump-incident')).toBe(false)
  })

  it('filters nav commands by label', () => {
    const results = buildCommandResults('graph', [], [])
    expect(results).toHaveLength(1)
    expect(results[0]).toMatchObject({ kind: 'nav', label: 'Go to Graph' })
  })

  it('offers a direct ID jump for an ID-shaped query, distinct from search results', () => {
    const results = buildCommandResults('inc-4f3a91', [], [])
    expect(results.some((r) => r.kind === 'jump-incident')).toBe(true)
    expect(results.some((r) => r.kind === 'jump-execution')).toBe(true)
  })

  it('does not offer an ID jump for a plain word query', () => {
    const results = buildCommandResults('payment', [], [])
    expect(results.some((r) => r.kind === 'jump-incident')).toBe(false)
  })

  it('does not offer an ID jump for a query containing spaces', () => {
    const results = buildCommandResults('atlas payment service', [], [])
    expect(results.some((r) => r.kind === 'jump-incident')).toBe(false)
  })

  it('searches only already-loaded incidents by title/service/id', () => {
    const incidents = [
      makeIncident('inc-1', { title: 'Payment degradation', rootService: 'atlas-payment-service' }),
      makeIncident('inc-2', { title: 'Order failure', rootService: 'atlas-order-service' }),
    ]
    const results = buildCommandResults('payment', incidents, [])
    const incidentResults = results.filter((r) => r.kind === 'incident')
    expect(incidentResults).toHaveLength(1)
  })

  it('searches only already-loaded executions, never fabricating a match', () => {
    const executions = [makeExecution('exec-1', { service: 'atlas-payment-service' }), makeExecution('exec-2', { service: 'atlas-order-service' })]
    const results = buildCommandResults('atlas-order', [], executions)
    const executionResults = results.filter((r) => r.kind === 'execution')
    expect(executionResults).toHaveLength(1)
  })

  it('returns nothing beyond nav for a query matching no loaded data', () => {
    const results = buildCommandResults('zzz-nonexistent', [makeIncident('inc-1')], [])
    expect(results.filter((r) => r.kind === 'incident')).toHaveLength(0)
    expect(results.filter((r) => r.kind === 'execution')).toHaveLength(0)
  })
})
