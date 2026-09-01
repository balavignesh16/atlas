// Timestamp formatting. One source of truth so relative/absolute time never
// drifts in presentation between pages. No date library dependency --
// native Intl is sufficient for what this app needs.

const RTF = new Intl.RelativeTimeFormat('en', { numeric: 'auto' })
const ABSOLUTE = new Intl.DateTimeFormat('en', {
  year: 'numeric',
  month: 'short',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
})

/** "4 minutes ago", "in 2 seconds", etc. */
export function relativeTime(iso: string | Date): string {
  const date = typeof iso === 'string' ? new Date(iso) : iso
  const diffSeconds = (date.getTime() - Date.now()) / 1000

  const divisions: [number, Intl.RelativeTimeFormatUnit][] = [
    [60, 'second'],
    [60, 'minute'],
    [24, 'hour'],
    [7, 'day'],
    [4.345, 'week'],
    [12, 'month'],
    [Number.POSITIVE_INFINITY, 'year'],
  ]

  let duration = diffSeconds
  for (const [amount, unit] of divisions) {
    if (Math.abs(duration) < amount) {
      return RTF.format(Math.round(duration), unit)
    }
    duration /= amount
  }
  return RTF.format(Math.round(duration), 'year')
}

/** Full absolute timestamp, for tooltips and detail views. */
export function absoluteTime(iso: string | Date): string {
  const date = typeof iso === 'string' ? new Date(iso) : iso
  return ABSOLUTE.format(date)
}

/** Compact elapsed duration since `iso`, e.g. "4m", "1h 12m", "2d". Used for
 * incident "age" columns -- always counts up from a real StartedAt/detected
 * timestamp, never a fabricated value. */
export function elapsedSince(iso: string | Date, now: Date = new Date()): string {
  const start = typeof iso === 'string' ? new Date(iso) : iso
  const totalSeconds = Math.max(0, Math.floor((now.getTime() - start.getTime()) / 1000))

  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  if (minutes > 0) return `${minutes}m`
  return `${seconds}s`
}

/** "just now" / "3s ago" style label for a last-poll indicator. */
export function secondsAgoLabel(date: Date, now: Date = new Date()): string {
  const seconds = Math.max(0, Math.floor((now.getTime() - date.getTime()) / 1000))
  if (seconds < 2) return 'just now'
  return `${seconds}s ago`
}
