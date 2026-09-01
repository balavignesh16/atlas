import { describe, expect, it } from 'vitest'
import type { Incident } from '@/api/types'
import { selectRecentIncidents } from './compose'

function makeIncident(id: string, startedAt: string): Incident {
  return {
    incidentId: id,
    status: 'OPEN',
    severity: 'CRITICAL',
    title: id,
    description: '',
    startedAt,
    lastUpdatedAt: startedAt,
    fingerprint: 'fp',
    rootService: 'svc',
    rootOperation: 'op',
    affectedServices: [],
    affectedOperations: [],
    affectedEdges: [],
    traceCount: 0,
    failureCount: 0,
    traceIds: [],
    evidenceIds: [],
    detectionReason: '',
  }
}

describe('selectRecentIncidents', () => {
  it('sorts newest-first by real startedAt', () => {
    const incidents = [
      makeIncident('a', '2026-01-01T00:00:00Z'),
      makeIncident('b', '2026-01-03T00:00:00Z'),
      makeIncident('c', '2026-01-02T00:00:00Z'),
    ]
    const result = selectRecentIncidents(incidents, 10)
    expect(result.map((i) => i.incidentId)).toEqual(['b', 'c', 'a'])
  })

  it('caps at the given limit rather than returning everything', () => {
    const incidents = Array.from({ length: 50 }, (_, i) => makeIncident(`inc-${i}`, `2026-01-01T00:00:${String(i).padStart(2, '0')}Z`))
    const result = selectRecentIncidents(incidents, 25)
    expect(result).toHaveLength(25)
  })

  it('does not mutate the input array', () => {
    const incidents = [makeIncident('a', '2026-01-01T00:00:00Z'), makeIncident('b', '2026-01-02T00:00:00Z')]
    const copy = [...incidents]
    selectRecentIncidents(incidents, 10)
    expect(incidents).toEqual(copy)
  })
})
