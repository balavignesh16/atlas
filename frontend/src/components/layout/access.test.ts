import { describe, expect, it } from 'vitest'
import type { Identity } from '@/api/auth'
import { describeAccess } from './access'

describe('describeAccess', () => {
  it('reports loading distinctly from every other state', () => {
    expect(describeAccess(undefined, true, false)).toEqual({ kind: 'loading' })
  })

  it('reports an error distinctly from security-disabled', () => {
    expect(describeAccess(undefined, false, true)).toEqual({ kind: 'error' })
  })

  it('reports security-disabled when securityEnabled is false, never fabricating an identity', () => {
    const identity: Identity = { securityEnabled: false }
    expect(describeAccess(identity, false, false)).toEqual({ kind: 'security-disabled' })
  })

  it('reports security-disabled when identity is undefined but not loading/erroring', () => {
    expect(describeAccess(undefined, false, false)).toEqual({ kind: 'security-disabled' })
  })

  it('reports the real identity when security is enabled', () => {
    const identity: Identity = { securityEnabled: true, name: 'alice', role: 'OPERATOR', permissions: ['VIEW'] }
    expect(describeAccess(identity, false, false)).toEqual({ kind: 'identity', name: 'alice', role: 'OPERATOR' })
  })

  it('never fabricates a name when the backend did not report one', () => {
    const identity: Identity = { securityEnabled: true, role: 'OPERATOR', permissions: ['VIEW'] }
    const result = describeAccess(identity, false, false)
    expect(result).toEqual({ kind: 'identity', name: undefined, role: 'OPERATOR' })
  })
})
