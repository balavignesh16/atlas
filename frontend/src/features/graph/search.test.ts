import { describe, expect, it } from 'vitest'
import { parseGraphSearch } from './search'

describe('parseGraphSearch', () => {
  it('parses a real string highlight param', () => {
    expect(parseGraphSearch({ highlight: 'inc-123' })).toEqual({ highlight: 'inc-123' })
  })

  it('resolves to undefined when the param is missing', () => {
    expect(parseGraphSearch({})).toEqual({ highlight: undefined })
  })

  it('resolves to undefined for a non-string value rather than coercing it', () => {
    expect(parseGraphSearch({ highlight: 42 })).toEqual({ highlight: undefined })
    expect(parseGraphSearch({ highlight: ['a', 'b'] })).toEqual({ highlight: undefined })
  })

  it('resolves an empty string to undefined', () => {
    expect(parseGraphSearch({ highlight: '' })).toEqual({ highlight: undefined })
  })
})
