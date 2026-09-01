import { Panel } from '@xyflow/react'
import { Button } from '@/components/ui/Button'
import { Tooltip } from '@/components/ui/Tooltip'

/**
 * `Panel` here is React Flow's own overlay-positioning primitive (distinct
 * from Atlas's `components/ui/Panel`) -- it anchors this toolbar to a
 * corner of the graph viewport so it visually belongs to the graph surface
 * rather than floating as an unrelated card.
 */
export function GraphToolbar({
  search,
  onSearchChange,
  onFitView,
  onReset,
  serviceCount,
}: {
  search: string
  onSearchChange: (value: string) => void
  onFitView: () => void
  onReset: () => void
  serviceCount: number
}) {
  return (
    <Panel position="top-left" className="!m-2">
      <div className="flex items-center gap-1.5 rounded-md border border-border-default bg-surface-1 p-1.5 shadow-lg">
        <input
          type="text"
          value={search}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="Search services…"
          aria-label="Search services"
          className="h-7 w-40 rounded-sm border border-border-subtle bg-surface-2 px-2 text-xs text-text-primary outline-none placeholder:text-text-disabled focus-visible:border-accent"
        />
        <Tooltip content="Fit view">
          <Button variant="ghost" size="sm" onClick={onFitView} aria-label="Fit view">
            ⤢
          </Button>
        </Tooltip>
        <Tooltip content="Reset search and view">
          <Button variant="ghost" size="sm" onClick={onReset} aria-label="Reset search and view">
            ↺
          </Button>
        </Tooltip>
        <span className="pl-1 text-2xs text-text-muted">{serviceCount} services</span>
      </div>
    </Panel>
  )
}
