import { describe, expect, it } from 'vitest'
import { canUse, permissionHint } from './permissions'
import type { PermissionSource } from './permissions'

describe('canUse', () => {
  it('never preemptively blocks before identity has loaded', () => {
    expect(canUse(undefined, 'EXECUTE')).toBe(true)
  })

  it('allows everything when security is disabled, since nothing is actually gated', () => {
    const identity: PermissionSource = { securityEnabled: false }
    expect(canUse(identity, 'APPROVE_PLAN')).toBe(true)
    expect(canUse(identity, 'EXECUTE')).toBe(true)
  })

  it('reflects the real permissions array when security is enabled', () => {
    const identity: PermissionSource = { securityEnabled: true, role: 'OPERATOR', permissions: ['VIEW', 'CREATE_PLAN'] }
    expect(canUse(identity, 'VIEW')).toBe(true)
    expect(canUse(identity, 'CREATE_PLAN')).toBe(true)
    expect(canUse(identity, 'APPROVE_PLAN')).toBe(false)
    expect(canUse(identity, 'EXECUTE')).toBe(false)
  })

  it('denies everything when security is enabled but no permissions were reported', () => {
    const identity: PermissionSource = { securityEnabled: true }
    expect(canUse(identity, 'VIEW')).toBe(false)
  })
})

describe('permissionHint', () => {
  it('returns null when the action is available', () => {
    const identity: PermissionSource = { securityEnabled: true, permissions: ['EXECUTE'] }
    expect(permissionHint(identity, 'EXECUTE')).toBeNull()
  })

  it('includes the real role when known', () => {
    const identity: PermissionSource = { securityEnabled: true, role: 'VIEWER', permissions: ['VIEW'] }
    expect(permissionHint(identity, 'APPROVE_PLAN')).toBe('Requires APPROVE_PLAN. Your current role: VIEWER.')
  })

  it('never fabricates a role when none is known', () => {
    const identity: PermissionSource = { securityEnabled: true, permissions: [] }
    expect(permissionHint(identity, 'EXECUTE')).toBe('Requires EXECUTE.')
  })

  it('returns null while identity is loading, matching canUse', () => {
    expect(permissionHint(undefined, 'EXECUTE')).toBeNull()
  })
})
