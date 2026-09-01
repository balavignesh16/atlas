import { ForbiddenError, NetworkError, UnauthorizedError } from '@/api/client'
import { Button } from './Button'

/** Distinguishes the real failure modes an operator can hit, per error
 * type, rather than one generic "something went wrong" message. */
export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  if (error instanceof UnauthorizedError) {
    return (
      <Panel title="Authentication required">
        <p>Your Atlas API key is invalid or has expired.</p>
      </Panel>
    )
  }

  if (error instanceof ForbiddenError) {
    return (
      <Panel title="Permission required">
        <p>
          This action requires{' '}
          <span className="font-mono text-text-primary">{error.requiredPermission ?? 'additional permissions'}</span>.
        </p>
      </Panel>
    )
  }

  if (error instanceof NetworkError) {
    return (
      <Panel
        title="Unable to reach Atlas"
        action={onRetry ? <Button size="sm" onClick={onRetry}>Retry</Button> : null}
      >
        <p>The intelligence engine did not respond.</p>
      </Panel>
    )
  }

  return (
    <Panel
      title="Something went wrong"
      action={onRetry ? <Button size="sm" onClick={onRetry}>Retry</Button> : null}
    >
      <p>{error instanceof Error ? error.message : 'An unexpected error occurred.'}</p>
    </Panel>
  )
}

function Panel({
  title,
  children,
  action,
}: {
  title: string
  children: React.ReactNode
  action?: React.ReactNode
}) {
  return (
    <div className="flex flex-col items-center gap-3 px-6 py-14 text-center">
      <p className="text-xs font-medium uppercase tracking-wide text-status-critical">{title}</p>
      <div className="max-w-sm text-xs text-text-muted">{children}</div>
      {action}
    </div>
  )
}
