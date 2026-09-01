import clsx from 'clsx'
import { TONE_CLASSES, toneFor } from './tone'

const ACTIVE_STATES = new Set(['EXECUTING', 'VERIFYING', 'PRECONDITION_CHECK'])

/** Status is always communicated via color + text together, never color
 * alone. The dot pulses only for genuinely in-progress states -- motion
 * that communicates something, not decoration. */
export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const tone = toneFor(status)
  const classes = TONE_CLASSES[tone]
  const isActive = ACTIVE_STATES.has(status)

  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1.5 rounded-sm px-1.5 py-0.5 text-2xs font-medium uppercase tracking-wide',
        classes.bg,
        classes.text,
      )}
    >
      <span className={clsx('h-1.5 w-1.5 rounded-full', classes.dot, isActive && 'animate-pulse')} />
      {label ?? status.replace(/_/g, ' ')}
    </span>
  )
}
