// Authentication state for the Atlas API key (X-Atlas-Api-Key).
//
// Deliberately sessionStorage, not localStorage: the key is a raw shared
// secret with no expiry and no token-exchange flow behind it (that is how
// the actual backend auth works today -- see internal/security/keystore.go),
// so this avoids persisting it across browser restarts. This is a disclosed
// trade-off, not a full solution to the backend's static-key model.
//
// Phase 1-4 had no way to honestly answer "who am I" or "what role am I"
// from an API key alone -- the backend had no identity endpoint, so the UI
// showed only a masked fragment of the key itself (see maskedKeyFragment).
// Phase 5 closes that gap with a small, read-only GET /api/v1/auth/me
// (internal/httpapi/auth.go) that returns exactly what the backend already
// resolved the key to -- see Identity/useIdentity below.

import { useQuery, type QueryClient } from '@tanstack/react-query'
import { useSyncExternalStore } from 'react'
import { apiFetch } from './client'
import type { Permission, Role } from '@/lib/permissions'

const STORAGE_KEY = 'atlas.apiKey'

type Listener = () => void
const listeners = new Set<Listener>()

function notify() {
  for (const listener of listeners) listener()
}

/**
 * Registered once at app startup (see app/providers.tsx) with the app's one
 * QueryClient instance, so clearSession can wipe cached query data too --
 * without this plain module needing to be a React hook or reach into the
 * provider tree. This is what actually prevents a previous principal's
 * incidents/executions/identity from surviving in cache for whoever
 * authenticates next in the same tab, whether the session ended via an
 * explicit sign-out or an automatic 401.
 */
let queryClientRef: QueryClient | null = null

export function registerQueryClient(client: QueryClient): void {
  queryClientRef = client
}

export function getApiKey(): string | null {
  return sessionStorage.getItem(STORAGE_KEY)
}

export function setApiKey(key: string): void {
  sessionStorage.setItem(STORAGE_KEY, key)
  notify()
}

/**
 * The single place a session ends, used by both an explicit sign-out and
 * apiFetch's automatic 401 handling -- so both paths get the same
 * guarantee: no stale principal-scoped data (identity, incidents,
 * executions, ...) left in the query cache for the next session in this
 * tab. A 403 (an authorization failure, not a session failure) and a
 * network error must NEVER call this -- see api/client.ts.
 */
export function clearSession(): void {
  sessionStorage.removeItem(STORAGE_KEY)
  queryClientRef?.clear()
  notify()
}

export function isAuthenticated(): boolean {
  return getApiKey() !== null
}

/** Last 4 characters of the key, for a non-sensitive "which key am I using"
 * display -- never the full key, never a fabricated name/role. */
export function maskedKeyFragment(key: string): string {
  if (key.length <= 4) return '•'.repeat(key.length)
  return `••••${key.slice(-4)}`
}

function subscribe(listener: Listener): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useAuth() {
  const apiKey = useSyncExternalStore(subscribe, getApiKey)
  return { apiKey, isAuthenticated: apiKey !== null }
}

/**
 * GET /api/v1/auth/me response, verified against internal/httpapi/auth.go.
 * `securityEnabled: false` means ATLAS_SECURITY_ENABLED is off backend-wide
 * -- in that state name/role/permissions are genuinely absent (nothing was
 * authenticated), not merely omitted, and must never be filled in with a
 * fabricated identity.
 */
export interface Identity {
  securityEnabled: boolean
  name?: string
  role?: Role
  permissions?: Permission[]
}

/**
 * Fetched once per session (keyed on the current API key, so a different
 * key after logout/login triggers a fresh fetch) rather than polled --
 * a principal's role does not change mid-session for a static API key.
 * A 401 here is handled by the same global mechanism every other endpoint
 * uses (apiFetch clears the session on any 401), so an invalid/expired key
 * is caught immediately after login rather than only on first real use.
 */
export function useIdentity() {
  const { apiKey } = useAuth()
  return useQuery({
    queryKey: ['identity', apiKey],
    queryFn: () => apiFetch<Identity>('/api/v1/auth/me'),
    enabled: apiKey !== null,
    retry: false,
    staleTime: Infinity,
  })
}
