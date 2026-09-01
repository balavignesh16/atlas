import { absoluteTime, relativeTime } from '@/lib/time'
import { Tooltip } from './Tooltip'

/** Relative time by default, exact absolute timestamp (monospace) on hover.
 * Never fabricates a time -- `value` must be a real ISO timestamp from the
 * API. */
export function Timestamp({ value, className }: { value: string; className?: string }) {
  return (
    <Tooltip content={<span className="font-mono">{absoluteTime(value)}</span>}>
      <time dateTime={value} className={className}>
        {relativeTime(value)}
      </time>
    </Tooltip>
  )
}
