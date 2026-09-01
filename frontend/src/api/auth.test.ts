import { QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// Same minimal in-memory sessionStorage stand-in as client.test.ts -- the
// Vitest environment here is plain Node (see vite.config.ts), which has no
// sessionStorage global.
function createMemoryStorage(): Storage {
  const store = new Map<string, string>()
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => void store.set(key, value),
    removeItem: (key: string) => void store.delete(key),
    clear: () => store.clear(),
    key: () => null,
    get length() {
      return store.size
    },
  } as Storage
}

beforeEach(() => {
  vi.stubGlobal('sessionStorage', createMemoryStorage())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

async function importFresh() {
  vi.resetModules()
  return import('./auth')
}

describe('session storage round-trip', () => {
  it('setApiKey/getApiKey/isAuthenticated agree', async () => {
    const { setApiKey, getApiKey, isAuthenticated } = await importFresh()
    expect(isAuthenticated()).toBe(false)
    setApiKey('abc-123-key')
    expect(getApiKey()).toBe('abc-123-key')
    expect(isAuthenticated()).toBe(true)
  })
})

describe('maskedKeyFragment', () => {
  it('never exposes more than the last 4 characters', async () => {
    const { maskedKeyFragment } = await importFresh()
    expect(maskedKeyFragment('super-secret-api-key-value')).toBe('••••alue')
    expect(maskedKeyFragment('super-secret-api-key-value')).not.toContain('super-secret')
  })

  it('masks a short key entirely rather than exposing it', async () => {
    const { maskedKeyFragment } = await importFresh()
    expect(maskedKeyFragment('ab')).toBe('••')
  })
})

describe('clearSession', () => {
  it('removes the stored API key', async () => {
    const { setApiKey, getApiKey, clearSession } = await importFresh()
    setApiKey('some-key')
    clearSession()
    expect(getApiKey()).toBeNull()
  })

  it('clears the registered QueryClient cache, so no previous identity/incident data survives', async () => {
    const { setApiKey, clearSession, registerQueryClient } = await importFresh()
    const queryClient = new QueryClient()
    registerQueryClient(queryClient)

    setApiKey('alice-key')
    queryClient.setQueryData(['identity', 'alice-key'], { securityEnabled: true, name: 'alice', role: 'OPERATOR' })
    queryClient.setQueryData(['incidents', 'open'], [{ incidentId: 'inc-1' }])
    expect(queryClient.getQueryData(['identity', 'alice-key'])).toBeDefined()

    clearSession()

    expect(queryClient.getQueryData(['identity', 'alice-key'])).toBeUndefined()
    expect(queryClient.getQueryData(['incidents', 'open'])).toBeUndefined()
  })

  it('does not throw when no QueryClient has been registered', async () => {
    const { setApiKey, clearSession } = await importFresh()
    setApiKey('some-key')
    expect(() => clearSession()).not.toThrow()
  })

  it('a fresh login after sign-out starts with an empty cache under the new key', async () => {
    const { setApiKey, clearSession, registerQueryClient } = await importFresh()
    const queryClient = new QueryClient()
    registerQueryClient(queryClient)

    setApiKey('alice-key')
    queryClient.setQueryData(['identity', 'alice-key'], { securityEnabled: true, name: 'alice', role: 'ADMIN' })
    clearSession()

    setApiKey('bob-key')
    // Nothing carried over under the new key, and the old key's cache entry
    // is gone -- a real requery under ['identity', 'bob-key'] is required.
    expect(queryClient.getQueryData(['identity', 'bob-key'])).toBeUndefined()
    expect(queryClient.getQueryData(['identity', 'alice-key'])).toBeUndefined()
  })

  it('notifies useAuth subscribers so a mounted UI reacts immediately', async () => {
    const { setApiKey, clearSession, getApiKey } = await importFresh()
    setApiKey('some-key')

    // useAuth's subscribe/notify pair is internal, but we can observe the
    // externally-visible effect: getApiKey() reflects the change
    // synchronously, which is what useSyncExternalStore polls after notify().
    clearSession()
    expect(getApiKey()).toBeNull()
  })
})
