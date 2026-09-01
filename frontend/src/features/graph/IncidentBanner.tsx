import { Panel } from '@xyflow/react'
import { Link } from '@tanstack/react-router'
import type { Incident } from '@/api/types'
import { Button } from '@/components/ui/Button'
import { SeverityIndicator } from '@/components/status/SeverityIndicator'

/** Shown only when /graph?highlight=<incidentId> resolves to a real
 * incident. Names the incident by its real title/ID -- never a generic
 * "Incident" label -- and gives a direct, single-click way back. */
export function IncidentBanner({ incident, onClear }: { incident: Incident; onClear: () => void }) {
  return (
    <Panel position="top-center" className="!m-2">
      <div className="flex items-center gap-3 rounded-md border border-border-default bg-surface-1 px-3 py-1.5 shadow-lg">
        <SeverityIndicator severity={incident.severity} />
        <span className="text-xs text-text-secondary">
          Viewing blast radius for <span className="font-medium text-text-primary">{incident.title}</span>
        </span>
        <Link to="/incidents/$incidentId" params={{ incidentId: incident.incidentId }}>
          <Button variant="secondary" size="sm">
            Open incident
          </Button>
        </Link>
        <Button variant="ghost" size="sm" onClick={onClear} aria-label="Clear incident highlight">
          Clear
        </Button>
      </div>
    </Panel>
  )
}
