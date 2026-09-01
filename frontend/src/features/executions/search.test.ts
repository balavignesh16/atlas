import { describe, expect, it } from 'vitest'
import { parseExecutionsSearch } from './search'

describe('parseExecutionsSearch', () => {
  it('parses a real string q param', () => {
    expect(parseExecutionsSearch({ q: 'payment' })).toEqual({ q: 'payment' })
  })

  it('resolves to undefined when missing', () => {
    expect(parseExecutionsSearch({})).toEqual({ q: undefined })
  })

  it('resolves to undefined for a non-string value', () => {
    expect(parseExecutionsSearch({ q: 42 })).toEqual({ q: undefined })
  })

  it('resolves an empty string to undefined', () => {
    expect(parseExecutionsSearch({ q: '' })).toEqual({ q: undefined })
  })
})
