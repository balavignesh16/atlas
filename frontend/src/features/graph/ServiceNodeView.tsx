import { Handle, Position, type NodeProps } from '@xyflow/react'
import clsx from 'clsx'
import type { HighlightState } from './highlight'

export interface ServiceNodeData {
  name: string
  highlight: HighlightState
  dimmed: boolean
  onOpen: () => void
  [key: string]: unknown
}

/**
 * Atlas's own compact node -- deliberately not React Flow's default card.
 * Communicates exactly two real things: the service name, and (only when an
 * incident is being highlighted) whether this service is affected/related/
 * unrelated to it. No fabricated health dot, no invented metric badge.
 * Affected/related state is never color-only: the label and border style
 * both change too.
 */
export function ServiceNodeView({ data, selected }: NodeProps & { data: ServiceNodeData }) {
  const { name, highlight, dimmed, onOpen } = data

  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={`Open ${name} details`}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onOpen()
        }
      }}
      className={clsx(
        'w-56 cursor-pointer rounded-md border bg-surface-1 px-3 py-2 transition-opacity',
        selected ? 'border-accent' : borderForHighlight(highlight),
        dimmed && 'opacity-35',
      )}
    >
      <Handle type="target" position={Position.Left} className="!h-1.5 !w-1.5 !border-none !bg-border-strong" />
      <div className="flex items-center gap-1.5">
        <span className={clsx('h-1.5 w-1.5 shrink-0 rounded-full', dotForHighlight(highlight))} aria-hidden="true" />
        <span className="truncate text-xs font-medium text-text-primary">{name}</span>
      </div>
      <p className="mt-0.5 text-2xs text-text-muted">
        service
        {highlight === 'affected' ? ' · affected' : null}
        {highlight === 'related' ? ' · related' : null}
      </p>
      <Handle type="source" position={Position.Right} className="!h-1.5 !w-1.5 !border-none !bg-border-strong" />
    </div>
  )
}

function borderForHighlight(highlight: HighlightState): string {
  if (highlight === 'affected') return 'border-status-critical'
  if (highlight === 'related') return 'border-status-warning'
  return 'border-border-default'
}

function dotForHighlight(highlight: HighlightState): string {
  if (highlight === 'affected') return 'bg-status-critical'
  if (highlight === 'related') return 'bg-status-warning'
  return 'bg-text-disabled'
}
