import { getSmoothStepPath, type EdgeProps } from '@xyflow/react'
import type { DependencyEdge } from '@/api/types'
import { colorVarFor, deriveErrorRate, formatErrorRate, widthForCallCount } from './edge-metrics'
import type { HighlightState } from './highlight'
import { Tooltip } from '@/components/ui/Tooltip'

export interface AtlasEdgeData {
  edge: DependencyEdge
  highlight: HighlightState
  dimmed: boolean
  onOpen: () => void
  [key: string]: unknown
}

export function DependencyEdgeView({
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  selected,
  markerEnd,
}: EdgeProps & { data: AtlasEdgeData }) {
  const { edge, highlight, dimmed, onOpen } = data
  const errorRate = deriveErrorRate(edge)
  const hasErrors = edge.error_count > 0

  const [path] = getSmoothStepPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
    borderRadius: 6,
  })

  const stroke = colorVarFor(highlight, hasErrors, Boolean(selected))
  const strokeWidth = selected ? widthForCallCount(edge.call_count) + 1 : widthForCallCount(edge.call_count)
  const opacity = dimmed ? 0.25 : hasErrors || highlight !== 'unrelated' ? 0.95 : 0.55

  return (
    <Tooltip
      content={
        <div className="min-w-40 space-y-0.5">
          <p className="mb-1 font-mono text-2xs text-text-primary">
            {edge.source} → {edge.target}
          </p>
          <TooltipRow label="Calls" value={edge.call_count.toLocaleString()} />
          <TooltipRow label="Errors" value={edge.error_count.toLocaleString()} />
          <TooltipRow label="Error rate" value={formatErrorRate(errorRate)} />
          <TooltipRow label="Avg latency" value={`${edge.average_duration_ms.toLocaleString()} ms`} />
        </div>
      }
    >
      {/* A wide, invisible interaction path sits under the thin visible
          stroke so hover/click targets more than a hairline -- the visible
          path alone would be nearly unhoverable at this stroke width. */}
      <g
        role="button"
        tabIndex={0}
        aria-label={`Inspect dependency from ${edge.source} to ${edge.target}`}
        onClick={onOpen}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onOpen()
          }
        }}
        className="cursor-pointer"
      >
        <path d={path} fill="none" stroke="transparent" strokeWidth={20} />
        <path d={path} fill="none" stroke={stroke} strokeWidth={strokeWidth} style={{ opacity }} markerEnd={markerEnd} />
      </g>
    </Tooltip>
  )
}

function TooltipRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-4 text-2xs">
      <span className="text-text-muted">{label}</span>
      <span className="font-mono text-text-primary">{value}</span>
    </div>
  )
}
