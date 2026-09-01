import { useNavigate } from '@tanstack/react-router'
import { useRef } from 'react'
import type { Incident } from '@/api/types'
import { SeverityIndicator } from '@/components/status/SeverityIndicator'
import { StatusBadge } from '@/components/status/StatusBadge'
import { elapsedSince } from '@/lib/time'
import type { IncidentDisplayState } from './incident-state'

interface IncidentTableProps {
  incidents: Incident[]
  /** Optional map of incidentId -> display state. When omitted, the
   * incident's own `status` field is shown -- see the Phase 1 report for
   * why the full Incidents page does not fetch the richer, execution-aware
   * state that the Command Center's open-incidents table does. */
  stateFor?: (incident: Incident) => IncidentDisplayState
}

export function IncidentTable({ incidents, stateFor }: IncidentTableProps) {
  const navigate = useNavigate()
  const rowRefs = useRef<Array<HTMLTableRowElement | null>>([])

  function open(incidentId: string) {
    navigate({ to: '/incidents/$incidentId', params: { incidentId } })
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTableRowElement>, index: number, incidentId: string) {
    if (e.key === 'Enter') {
      open(incidentId)
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      rowRefs.current[index + 1]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      rowRefs.current[index - 1]?.focus()
    }
  }

  return (
    <table className="w-full text-left">
      <thead>
        <tr className="border-b border-border-default text-2xs uppercase tracking-wide text-text-muted">
          <th className="w-12 px-3 py-2 font-medium">Sev</th>
          <th className="px-3 py-2 font-medium">Service</th>
          <th className="px-3 py-2 font-medium">Operation</th>
          <th className="w-20 px-3 py-2 font-medium">Age</th>
          <th className="w-40 px-3 py-2 font-medium">State</th>
        </tr>
      </thead>
      <tbody>
        {incidents.map((incident, index) => {
          const state = stateFor ? stateFor(incident) : incident.status
          return (
            <tr
              key={incident.incidentId}
              ref={(el) => {
                rowRefs.current[index] = el
              }}
              tabIndex={0}
              role="row"
              aria-label={`Incident: ${incident.title}`}
              onClick={() => open(incident.incidentId)}
              onKeyDown={(e) => onKeyDown(e, index, incident.incidentId)}
              className="cursor-pointer border-b border-border-subtle text-xs text-text-primary outline-none transition-colors hover:bg-surface-2 focus-visible:bg-surface-2"
            >
              <td className="px-3 py-2.5">
                <SeverityIndicator severity={incident.severity} />
              </td>
              <td className="px-3 py-2.5 font-medium">{incident.rootService}</td>
              <td className="px-3 py-2.5 text-text-secondary">{incident.rootOperation}</td>
              <td className="px-3 py-2.5 font-mono text-2xs text-text-secondary">
                {elapsedSince(incident.startedAt)}
              </td>
              <td className="px-3 py-2.5">
                <StatusBadge status={state} />
              </td>
            </tr>
          )
        })}
      </tbody>
    </table>
  )
}
