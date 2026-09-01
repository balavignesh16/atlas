// Mirrors internal/security/model.go's Role/Permission constants exactly
// (kept as TS string-literal unions purely for typing).
//
// Phase 1 shipped a hardcoded role->permission table here as a UX
// convenience, but it was never actually wired into any control across
// Phases 1-4 (grep confirms it). Phase 5 replaces it entirely: the backend
// now exposes the authenticated principal's REAL permission set via
// GET /api/v1/auth/me (see api/auth.ts's useIdentity), so checking against
// that real data is strictly better than re-deriving it from a second,
// hand-maintained copy of the backend's mapping that could silently drift.
//
// This remains a UX convenience ONLY -- see canUse's own doc comment. The
// backend re-checks every permission on every real request regardless of
// what this file says, and its rejection (403) is always authoritative.

export type Role = 'VIEWER' | 'OPERATOR' | 'APPROVER' | 'EXECUTOR' | 'ADMIN'
export type Permission = 'VIEW' | 'CREATE_PLAN' | 'APPROVE_PLAN' | 'EXECUTE' | 'READ_AUDIT'

export const ALL_PERMISSIONS: readonly Permission[] = ['VIEW', 'CREATE_PLAN', 'APPROVE_PLAN', 'EXECUTE', 'READ_AUDIT']

/** The minimal shape canUse/permissionHint need -- deliberately not
 * importing api/auth's full Identity type here, to keep this module
 * dependency-free and avoid a circular import (Identity is defined in
 * terms of Role/Permission from this file). Identity satisfies this shape
 * structurally. */
export interface PermissionSource {
  securityEnabled: boolean
  role?: Role
  permissions?: Permission[]
}

/**
 * Never preemptively blocks:
 *  - identity not loaded yet -> true (unknown, not denied; if the action
 *    truly isn't allowed, the real request will come back 403)
 *  - security disabled -> true (nothing is actually gated backend-side, so
 *    showing a control as unavailable here would be dishonest)
 *  - security enabled -> reflects the real `permissions` array from
 *    GET /api/v1/auth/me
 */
export function canUse(identity: PermissionSource | undefined, permission: Permission): boolean {
  if (!identity) return true
  if (!identity.securityEnabled) return true
  return identity.permissions?.includes(permission) ?? false
}

/** Human explanation for a disabled control, or null when the action is
 * actually available. Never fabricates a role when the backend didn't
 * report one. */
export function permissionHint(identity: PermissionSource | undefined, permission: Permission): string | null {
  if (canUse(identity, permission)) return null
  if (identity?.role) return `Requires ${permission}. Your current role: ${identity.role}.`
  return `Requires ${permission}.`
}
