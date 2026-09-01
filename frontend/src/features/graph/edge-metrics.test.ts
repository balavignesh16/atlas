import { describe, expect, it } from 'vitest'
import { colorVarFor, deriveErrorRate, edgeKey, formatErrorRate, widthForCallCount } from './edge-metrics'

describe('deriveErrorRate', () => {
  it('divides error_count by call_count', () => {
    expect(deriveErrorRate({ call_count: 100, error_count: 25 })).toBe(0.25)
  })

  it('returns null when call_count is 0, never a fabricated 0%', () => {
    expect(deriveErrorRate({ call_count: 0, error_count: 0 })).toBeNull()
  })

  it('handles a call_count with no errors', () => {
    expect(deriveErrorRate({ call_count: 50, error_count: 0 })).toBe(0)
  })
})

describe('formatErrorRate', () => {
  it('formats as a percentage with one decimal', () => {
    expect(formatErrorRate(0.063)).toBe('6.3%')
  })

  it('renders an em dash for null, never "0%" or "NaN%"', () => {
    expect(formatErrorRate(null)).toBe('—')
  })
})

describe('edgeKey', () => {
  it('matches the backend graph.go edgeKey format exactly', () => {
    expect(edgeKey('atlas-gateway', 'atlas-order-service')).toBe('atlas-gateway->atlas-order-service')
  })
})

describe('widthForCallCount', () => {
  it('is monotonically non-decreasing with call count', () => {
    const widths = [0, 1, 10, 100, 1000, 10000].map(widthForCallCount)
    for (let i = 1; i < widths.length; i++) {
      expect(widths[i]).toBeGreaterThanOrEqual(widths[i - 1])
    }
  })

  it('is capped so one hot edge cannot dominate the graph', () => {
    expect(widthForCallCount(1_000_000)).toBeLessThanOrEqual(4)
  })
})

describe('colorVarFor', () => {
  it('prioritizes selection over highlight state', () => {
    expect(colorVarFor('affected', true, true)).toBe('var(--color-accent)')
  })

  it('uses the critical tone for affected edges', () => {
    expect(colorVarFor('affected', false, false)).toBe('var(--color-status-critical)')
  })

  it('uses the failed tone for edges with real errors when not part of a highlight', () => {
    expect(colorVarFor('none', true, false)).toBe('var(--color-status-failed)')
  })

  it('falls back to the restrained default for a healthy, unhighlighted edge', () => {
    expect(colorVarFor('none', false, false)).toBe('var(--color-border-strong)')
  })
})
