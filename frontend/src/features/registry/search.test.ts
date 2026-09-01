import { describe, expect, it } from 'vitest'
import { parseServicesSearch } from './search'

describe('parseServicesSearch', () => {
  it('parses valid status/source/q values', () => {
    expect(parseServicesSearch({ status: 'ACTIVE', source: 'OBSERVED_TELEMETRY', q: 'checkout' })).toEqual({
      status: 'ACTIVE',
      source: 'OBSERVED_TELEMETRY',
      q: 'checkout',
    })
  })

  it('resolves missing params to undefined', () => {
    expect(parseServicesSearch({})).toEqual({ status: undefined, source: undefined, q: undefined })
  })

  it('resolves an invalid status value to undefined rather than passing it through', () => {
    expect(parseServicesSearch({ status: 'NOT_A_REAL_STATUS' }).status).toBeUndefined()
  })

  it('resolves an invalid source value to undefined rather than passing it through', () => {
    expect(parseServicesSearch({ source: 'NOT_A_REAL_SOURCE' }).source).toBeUndefined()
  })

  it('resolves an empty q to undefined', () => {
    expect(parseServicesSearch({ q: '' }).q).toBeUndefined()
  })

  it('resolves a non-string q to undefined', () => {
    expect(parseServicesSearch({ q: 42 }).q).toBeUndefined()
  })

  it('accepts every real status value', () => {
    for (const status of ['ACTIVE', 'STALE', 'RETIRED']) {
      expect(parseServicesSearch({ status }).status).toBe(status)
    }
  })

  it('accepts every real source value', () => {
    for (const source of ['OBSERVED_TELEMETRY', 'DECLARED', 'DOCKER', 'KUBERNETES', 'CONFIG', 'INFERRED']) {
      expect(parseServicesSearch({ source }).source).toBe(source)
    }
  })
})
