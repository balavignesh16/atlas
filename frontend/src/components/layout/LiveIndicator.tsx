import { useEffect, useState } from 'react'
import { secondsAgoLabel } from '@/lib/time'

/**
 * "● LIVE / Updated Xs ago" while recent queries are succeeding, or
 * "● CONNECTION DEGRADED / Last updated Xs ago" once a real request has
 * failed with a network error. Never claims "LIVE" while data is stale.
 */
export function LiveIndicator({
  degraded,
  lastSuccessAt,
}: {
  degraded: boolean
  lastSuccessAt: Date | null
}) {
  const [, forceTick] = useState(0)

  useEffect(() => {
    const id = setInterval(() => forceTick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [])

  if (!lastSuccessAt) {
    return <span className="text-2xs uppercase tracking-wide text-text-muted">Connecting…</span>
  }

  if (degraded) {
    return (
      <span className="flex items-center gap-1.5 text-2xs uppercase tracking-wide text-status-critical">
        <span className="h-1.5 w-1.5 rounded-full bg-status-critical" />
        Connection degraded
        <span className="text-text-muted normal-case">· last update {secondsAgoLabel(lastSuccessAt)}</span>
      </span>
    )
  }

  return (
    <span className="flex items-center gap-1.5 text-2xs uppercase tracking-wide text-status-healthy">
      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-status-healthy" />
      Live
      <span className="text-text-muted normal-case">· updated {secondsAgoLabel(lastSuccessAt)}</span>
    </span>
  )
}
