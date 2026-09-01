import { QueryClient } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// apiFetch reads/writes sessionStorage indirectly via getApiKey/clearSession
// (api/auth.ts). The Vitest environment here is plain Node (see
// vite.config.ts), which has no sessionStorage global, so a minimal
// in-memory stand-in is installed rather than adding a jsdom/happy-dom
// dependency just for this.
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
  return import('./client')
}

describe('apiFetch: 401 handling', () => {
  it('clears the session and throws UnauthorizedError on a real 401', async () => {
    sessionStorage.setItem('atlas.apiKey', 'some-now-invalid-key')

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ error: 'invalid API key' }), { status: 401 })),
    )

    const { apiFetch, UnauthorizedError } = await importFresh()
    await expect(apiFetch('/api/v1/auth/me')).rejects.toBeInstanceOf(UnauthorizedError)
    expect(sessionStorage.getItem('atlas.apiKey')).toBeNull()
  })
})

describe('apiFetch: 403 handling', () => {
  it('extracts the required permission from a JSON body', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(JSON.stringify({ error: 'principal does not have permission: APPROVE_PLAN' }), { status: 403 }),
      ),
    )

    const { apiFetch, ForbiddenError } = await importFresh()
    try {
      await apiFetch('/api/v1/remediation/plan-1/approve')
      expect.unreachable('expected apiFetch to throw')
    } catch (err) {
      expect(err).toBeInstanceOf(ForbiddenError)
      expect((err as InstanceType<typeof ForbiddenError>).requiredPermission).toBe('APPROVE_PLAN')
    }
  })

  it('has no requiredPermission for a guard/business-rule 403', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ error: 'plan is not in APPROVED status' }), { status: 403 })),
    )

    const { apiFetch, ForbiddenError } = await importFresh()
    try {
      await apiFetch('/api/v1/remediation/plan-1/execute')
      expect.unreachable('expected apiFetch to throw')
    } catch (err) {
      expect(err).toBeInstanceOf(ForbiddenError)
      expect((err as InstanceType<typeof ForbiddenError>).requiredPermission).toBeUndefined()
      expect((err as Error).message).toBe('plan is not in APPROVED status')
    }
  })
})

describe('apiFetch: non-JSON error bodies', () => {
  it('preserves a plain-text error body as the message rather than a generic fallback', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response('cannot approve plan in status: PROPOSED', {
            status: 400,
            headers: { 'Content-Type': 'text/plain; charset=utf-8' },
          }),
      ),
    )

    const { apiFetch, ApiError } = await importFresh()
    try {
      await apiFetch('/api/v1/remediation/plan-1/approve')
      expect.unreachable('expected apiFetch to throw')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError)
      expect((err as Error).message).toBe('cannot approve plan in status: PROPOSED')
    }
  })
})

describe('apiFetch: session/cache isolation (Phase 6)', () => {
  async function importFreshWithAuth() {
    vi.resetModules()
    const client = await import('./client')
    const auth = await import('./auth')
    return { ...client, ...auth }
  }

  it('a 401 clears both the session AND the registered query cache', async () => {
    const { apiFetch, registerQueryClient, setApiKey } = await importFreshWithAuth()
    const queryClient = new QueryClient()
    registerQueryClient(queryClient)
    setApiKey('now-invalid-key')
    queryClient.setQueryData(['identity', 'now-invalid-key'], { securityEnabled: true, role: 'VIEWER' })

    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'invalid API key' }), { status: 401 })))

    await expect(apiFetch('/api/v1/incidents/open')).rejects.toThrow()
    expect(sessionStorage.getItem('atlas.apiKey')).toBeNull()
    expect(queryClient.getQueryData(['identity', 'now-invalid-key'])).toBeUndefined()
  })

  it('a 403 (authorization failure) does NOT clear the session or the cache', async () => {
    const { apiFetch, registerQueryClient, setApiKey } = await importFreshWithAuth()
    const queryClient = new QueryClient()
    registerQueryClient(queryClient)
    setApiKey('valid-but-limited-key')
    queryClient.setQueryData(['identity', 'valid-but-limited-key'], { securityEnabled: true, role: 'VIEWER' })

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ error: 'principal does not have permission: EXECUTE' }), { status: 403 })),
    )

    await expect(apiFetch('/api/v1/remediation/plan-1/execute')).rejects.toThrow()
    expect(sessionStorage.getItem('atlas.apiKey')).toBe('valid-but-limited-key')
    expect(queryClient.getQueryData(['identity', 'valid-but-limited-key'])).toBeDefined()
  })

  it('a network failure does NOT clear the session merely because the network is unavailable', async () => {
    const { apiFetch, registerQueryClient, setApiKey } = await importFreshWithAuth()
    const queryClient = new QueryClient()
    registerQueryClient(queryClient)
    setApiKey('a-perfectly-valid-key')
    queryClient.setQueryData(['identity', 'a-perfectly-valid-key'], { securityEnabled: true, role: 'ADMIN' })

    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('fetch failed')
      }),
    )

    const { NetworkError } = await import('./client')
    await expect(apiFetch('/api/v1/incidents/open')).rejects.toBeInstanceOf(NetworkError)
    expect(sessionStorage.getItem('atlas.apiKey')).toBe('a-perfectly-valid-key')
    expect(queryClient.getQueryData(['identity', 'a-perfectly-valid-key'])).toBeDefined()
  })
})

describe('apiFetch: success', () => {
  it('returns the parsed JSON body on 200', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ securityEnabled: false }), { status: 200 })))

    const { apiFetch } = await importFresh()
    const result = await apiFetch<{ securityEnabled: boolean }>('/api/v1/auth/me')
    expect(result).toEqual({ securityEnabled: false })
  })

  it('returns undefined for a 204 with no body', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })))

    const { apiFetch } = await importFresh()
    const result = await apiFetch('/api/v1/something')
    expect(result).toBeUndefined()
  })
})
