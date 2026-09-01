import { describe, expect, it } from 'vitest'
import type { Service } from '@/api/types'
import { sortServices } from './sort'

function makeService(name: string, status: Service['status']): Service {
  return {
    name,
    displayName: name,
    provenance: 'OBSERVED_TELEMETRY',
    confidence: 'OBSERVED',
    status,
    firstObservedAt: '2026-01-01T00:00:00Z',
    lastObservedAt: '2026-01-01T00:00:00Z',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
  }
}

describe('sortServices', () => {
  it('orders ACTIVE before STALE before RETIRED', () => {
    const services = [makeService('c', 'RETIRED'), makeService('a', 'ACTIVE'), makeService('b', 'STALE')]
    expect(sortServices(services).map((s) => s.status)).toEqual(['ACTIVE', 'STALE', 'RETIRED'])
  })

  it('sorts alphabetically within the same status', () => {
    const services = [makeService('zeta', 'ACTIVE'), makeService('alpha', 'ACTIVE')]
    expect(sortServices(services).map((s) => s.name)).toEqual(['alpha', 'zeta'])
  })

  it('does not mutate the input array', () => {
    const services = [makeService('b', 'STALE'), makeService('a', 'ACTIVE')]
    const copy = [...services]
    sortServices(services)
    expect(services).toEqual(copy)
  })
})
