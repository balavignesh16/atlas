import clsx from 'clsx'
import type { IncidentSeverity } from '@/api/types'
import { TONE_CLASSES, toneFor } from './tone'

const LABEL: Record<IncidentSeverity, string> = {
  CRITICAL: 'CRIT',
  WARNING: 'WARN',
  INFO: 'INFO',
}

/** Compact severity marker for dense table rows -- a short mono-width label
 * plus color, so severity scans as a column at a glance. */
export function SeverityIndicator({ severity }: { severity: IncidentSeverity }) {
  const classes = TONE_CLASSES[toneFor(severity)]
  return (
    <span className={clsx('font-mono text-2xs font-semibold tracking-wide', classes.text)}>
      {LABEL[severity]}
    </span>
  )
}
