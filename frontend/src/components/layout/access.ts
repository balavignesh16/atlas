import type { Identity } from '@/api/auth'

export type AccessDescription =
  | { kind: 'loading' }
  | { kind: 'error' }
  | { kind: 'security-disabled' }
  | { kind: 'identity'; name?: string; role?: string }

/**
 * Pure decision logic for what the identity area should show, extracted so
 * it's testable without rendering the popover. Four genuinely different
 * states, never collapsed into one: loading is not "no identity", an
 * unexpected fetch error is not "security disabled", and security being
 * disabled is not a fabricated identity.
 */
export function describeAccess(identity: Identity | undefined, isPending: boolean, isError: boolean): AccessDescription {
  if (isPending) return { kind: 'loading' }
  if (isError) return { kind: 'error' }
  if (!identity || !identity.securityEnabled) return { kind: 'security-disabled' }
  return { kind: 'identity', name: identity.name, role: identity.role }
}
