import { useState } from 'react'
import { Tooltip } from './Tooltip'

/** A monospace, copyable machine identifier (execution ID, trace ID,
 * fingerprint, ...). Truncates long IDs visually but always copies the
 * full, real value -- never a shortened/fabricated one. */
export function IdChip({ value, truncate = 8 }: { value: string; truncate?: number }) {
  const [copied, setCopied] = useState(false)
  const display = value.length > truncate * 2 ? `${value.slice(0, truncate)}…${value.slice(-4)}` : value

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    } catch {
      // clipboard API unavailable -- fail silently, nothing to fabricate here
    }
  }

  return (
    <Tooltip content={copied ? 'Copied' : value}>
      <button
        type="button"
        onClick={handleCopy}
        className="rounded-sm px-1 py-0.5 font-mono text-2xs text-text-secondary hover:bg-surface-2 hover:text-text-primary"
      >
        {display}
      </button>
    </Tooltip>
  )
}
