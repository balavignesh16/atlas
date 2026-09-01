import * as Popover from '@radix-ui/react-popover'
import { useNavigate } from '@tanstack/react-router'
import { clearSession, maskedKeyFragment, useAuth, useIdentity } from '@/api/auth'
import { Button } from '@/components/ui/Button'
import { ALL_PERMISSIONS, canUse } from '@/lib/permissions'
import { describeAccess, type AccessDescription } from './access'

/**
 * Compact identity/permissions popover backed by the real
 * GET /api/v1/auth/me response -- never a fabricated name, role, or
 * permission list. See access.ts's describeAccess for the four states this
 * renders (loading / error / security-disabled / identity).
 */
export function AccessPopover() {
  const { apiKey } = useAuth()
  const { data: identity, isPending, isError } = useIdentity()

  if (!apiKey) return null

  const access = describeAccess(identity, isPending, isError)

  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="flex items-center gap-2 rounded-sm border border-border-default bg-surface-2 px-2.5 py-1 text-xs text-text-secondary outline-none hover:border-border-strong hover:text-text-primary focus-visible:border-accent"
          aria-label="Access and permissions"
        >
          <TriggerLabel access={access} />
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          sideOffset={8}
          align="end"
          className="z-50 w-64 rounded-md border border-border-default bg-surface-1 p-3 text-xs shadow-2xl outline-none"
        >
          <PopoverBody apiKey={apiKey} access={access} identity={identity} />
          <SignOutButton />
          <Popover.Arrow className="fill-border-default" />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

function TriggerLabel({ access }: { access: AccessDescription }) {
  if (access.kind === 'loading') return <span className="text-text-muted">Loading…</span>
  if (access.kind === 'error') return <span className="text-status-warning">Access unavailable</span>
  if (access.kind === 'security-disabled') return <span>Security disabled</span>
  return (
    <span>
      {access.role ?? 'Unknown role'}
      {access.name ? <span className="text-text-muted"> · {access.name}</span> : null}
    </span>
  )
}

function PopoverBody({
  apiKey,
  access,
  identity,
}: {
  apiKey: string
  access: AccessDescription
  identity: ReturnType<typeof useIdentity>['data']
}) {
  if (access.kind === 'loading') {
    return <p className="text-text-muted">Loading access information…</p>
  }

  if (access.kind === 'error') {
    return (
      <p className="text-text-muted">
        Could not load access information for this session. Actions are still subject to normal backend
        authorization.
      </p>
    )
  }

  if (access.kind === 'security-disabled') {
    return (
      <div className="space-y-2">
        <p className="font-medium text-text-primary">Security disabled</p>
        <p className="text-text-muted">
          ATLAS_SECURITY_ENABLED is off for this deployment. No principal is authenticated and every request is
          currently unrestricted.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div>
        <p className="text-sm font-semibold text-text-primary">{access.name ?? 'Unnamed principal'}</p>
        <p className="text-2xs uppercase tracking-wide text-text-muted">{access.role ?? 'Unknown role'}</p>
      </div>

      <div>
        <p className="mb-1 text-2xs uppercase tracking-wide text-text-muted">Permissions</p>
        <ul className="space-y-0.5">
          {ALL_PERMISSIONS.map((permission) => {
            const granted = canUse(identity, permission)
            return (
              <li key={permission} className="flex items-center gap-1.5">
                <span aria-hidden="true" className={granted ? 'text-status-healthy' : 'text-text-disabled'}>
                  {granted ? '✓' : '—'}
                </span>
                <span className={granted ? 'font-mono text-text-primary' : 'font-mono text-text-disabled'}>
                  {permission}
                </span>
                <span className="sr-only">{granted ? 'granted' : 'not granted'}</span>
              </li>
            )
          })}
        </ul>
      </div>

      <p className="border-t border-border-subtle pt-2 font-mono text-2xs text-text-disabled">
        Session key {maskedKeyFragment(apiKey)}
      </p>
    </div>
  )
}

/**
 * Always rendered whenever the popover itself renders (i.e. whenever a
 * session key exists at all -- see AccessPopover's early return), covering
 * every access.kind including "security-disabled": ending the browser
 * session is meaningful even with no principal, since a key is still
 * stored. clearSession() (api/auth.ts) clears sessionStorage AND the whole
 * query cache in one place; this handler only adds the navigation, which
 * the plain auth module correctly has no business doing itself.
 */
function SignOutButton() {
  const navigate = useNavigate()

  function handleSignOut() {
    clearSession()
    navigate({ to: '/login' })
  }

  return (
    <div className="mt-3 border-t border-border-subtle pt-3">
      <Button variant="secondary" size="sm" className="w-full" onClick={handleSignOut}>
        Sign out
      </Button>
    </div>
  )
}
