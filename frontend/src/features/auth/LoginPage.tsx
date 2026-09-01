import { useNavigate } from '@tanstack/react-router'
import { useState, type FormEvent } from 'react'
import { setApiKey } from '@/api/auth'
import { apiFetch, UnauthorizedError } from '@/api/client'
import { Button } from '@/components/ui/Button'

/**
 * The backend authenticates via a single static API key
 * (X-Atlas-Api-Key) -- there is no username/password, no OAuth, and no
 * session exchange. This screen reflects that reality exactly rather than
 * implying a richer auth model than actually exists.
 */
export function LoginPage() {
  const [key, setKey] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const navigate = useNavigate()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!key.trim()) return
    setSubmitting(true)
    setError(null)
    setApiKey(key.trim())
    try {
      // The real identity endpoint (Phase 5) doubles as key validation --
      // the cheapest genuine call that proves the key works. This is a
      // plain fetch, not a useQuery call, so it does not itself populate
      // the TopBar's identity cache; that component fetches its own copy
      // via useIdentity() once mounted, a second (equally cheap) request.
      await apiFetch('/api/v1/auth/me')
      navigate({ to: '/' })
    } catch (err) {
      if (err instanceof UnauthorizedError) {
        setError('That API key was rejected by Atlas.')
      } else {
        // Security may be disabled backend-side (ATLAS_SECURITY_ENABLED=false),
        // in which case any key value succeeds -- or the engine may simply be
        // unreachable. Either way, a non-auth error here still means the key
        // itself is accepted for now; let the user in and let normal
        // per-page error states handle connectivity.
        navigate({ to: '/' })
        return
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex h-full items-center justify-center bg-surface-0">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-sm rounded-lg border border-border-default bg-surface-1 p-6"
      >
        <p className="text-base font-semibold tracking-tight text-text-primary">Atlas</p>
        <p className="mb-6 text-xs text-text-muted">Operations Console</p>

        <label className="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-text-secondary">
          API Key
        </label>
        <input
          type="password"
          autoFocus
          value={key}
          onChange={(e) => setKey(e.target.value)}
          className="mb-3 w-full rounded-sm border border-border-default bg-surface-2 px-2.5 py-1.5 font-mono text-xs text-text-primary outline-none focus-visible:border-accent"
          placeholder="X-Atlas-Api-Key"
        />

        {error ? <p className="mb-3 text-2xs text-status-critical">{error}</p> : null}

        <Button type="submit" variant="primary" className="w-full" disabled={submitting || !key.trim()}>
          {submitting ? 'Connecting…' : 'Connect'}
        </Button>
      </form>
    </div>
  )
}
