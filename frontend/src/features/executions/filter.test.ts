import { describe, expect, it } from 'vitest'
import type { ExecutionRecord } from '@/api/types'
import { filterExecutions, sortExecutionsByStartedAt } from './filter'

function makeRecord(overrides: Partial<ExecutionRecord>): ExecutionRecord {
  return {
    executionId: 'exec-1',
    planId: 'plan-1',
    actionId: 'action-1',
    incidentId: 'inc-1',
    service: 'atlas-payment-service',
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

describe('filterExecutions', () => {
  const records = [
    makeRecord({ executionId: 'exec-a', service: 'atlas-payment-service', action: 'RESTART_SERVICE' }),
    makeRecord({ executionId: 'exec-b', service: 'atlas-order-service', action: 'SCALE_UP', incidentId: 'inc-2' }),
  ]

  it('returns everything for an empty query', () => {
    expect(filterExecutions(records, '')).toHaveLength(2)
  })

  it('matches by service', () => {
    const result = filterExecutions(records, 'order')
    expect(result.map((r) => r.executionId)).toEqual(['exec-b'])
  })

  it('matches by action', () => {
    const result = filterExecutions(records, 'restart')
    expect(result.map((r) => r.executionId)).toEqual(['exec-a'])
  })

  it('matches by incident ID', () => {
    const result = filterExecutions(records, 'inc-2')
    expect(result.map((r) => r.executionId)).toEqual(['exec-b'])
  })

  it('matches by execution ID', () => {
    const result = filterExecutions(records, 'exec-a')
    expect(result).toHaveLength(1)
  })

  it('is case-insensitive', () => {
    expect(filterExecutions(records, 'ATLAS-ORDER')).toHaveLength(1)
  })
})

describe('sortExecutionsByStartedAt', () => {
  it('sorts newest-first', () => {
    const records = [
      makeRecord({ executionId: 'a', startedAt: '2026-01-01T00:00:00Z' }),
      makeRecord({ executionId: 'b', startedAt: '2026-01-03T00:00:00Z' }),
      makeRecord({ executionId: 'c', startedAt: '2026-01-02T00:00:00Z' }),
    ]
    expect(sortExecutionsByStartedAt(records).map((r) => r.executionId)).toEqual(['b', 'c', 'a'])
  })
})
