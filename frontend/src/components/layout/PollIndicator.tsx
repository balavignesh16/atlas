import { useEffect, useState } from 'react'
import { secondsAgoLabel } from '@/lib/time'

/**
 * Page-local polling indicator, distinct from the global TopBar connection
 * badge (which reflects overall reachability across every query). This one
 * answers a narrower, honest question: is this page's own data still being
 * actively refreshed, and how stale is what's on screen right now. Never
 * shows "LIVE" once polling has stopped.
 */
export function PollIndicator({ polling, lastUpdatedAt }: { polling: boolean; lastUpdatedAt: number }) {
  const [, forceTick] = useState(0)

  useEffect(() => {
    if (!polling) return
    const id = setInterval(() => forceTick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [polling])

  if (!lastUpdatedAt) return null

  return (
    <span className="flex shrink-0 items-center gap-1.5 text-2xs uppercase tracking-wide text-text-muted">
      {polling ? (
        <>
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-status-healthy" />
          <span className="text-status-healthy">Live</span>
        </>
      ) : null}
      <span className="normal-case">
        {polling ? '· ' : ''}
        Updated {secondsAgoLabel(new Date(lastUpdatedAt))}
      </span>
    </span>
  )
}
