import { useNavigate } from '@tanstack/react-router'
import { useRef } from 'react'
import type { Service } from '@/api/types'
import { StatusBadge } from '@/components/status/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { describeProvenance } from './provenance'

/** Dense table matching IncidentTable/ExecutionTable conventions. Status
 * here is the registry's own lifecycle (ACTIVE/STALE/RETIRED), never to be
 * confused with whether the service currently has live graph edges. */
export function ServiceTable({ services }: { services: Service[] }) {
  const navigate = useNavigate()
  const rowRefs = useRef<Array<HTMLTableRowElement | null>>([])

  function open(name: string) {
    navigate({ to: '/services/$serviceName', params: { serviceName: name } })
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTableRowElement>, index: number, name: string) {
    if (e.key === 'Enter') {
      open(name)
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
          <th className="px-3 py-2 font-medium">Service</th>
          <th className="w-28 px-3 py-2 font-medium">Status</th>
          <th className="w-40 px-3 py-2 font-medium">Provenance</th>
          <th className="w-32 px-3 py-2 font-medium">First observed</th>
          <th className="w-32 px-3 py-2 font-medium">Last telemetry</th>
        </tr>
      </thead>
      <tbody>
        {services.map((service, index) => (
          <tr
            key={service.name}
            ref={(el) => {
              rowRefs.current[index] = el
            }}
            tabIndex={0}
            role="row"
            aria-label={`Service ${service.name}, ${service.status}`}
            onClick={() => open(service.name)}
            onKeyDown={(e) => onKeyDown(e, index, service.name)}
            className="cursor-pointer border-b border-border-subtle text-xs text-text-primary outline-none transition-colors hover:bg-surface-2 focus-visible:bg-surface-2"
          >
            <td className="px-3 py-2.5 font-medium">{service.name}</td>
            <td className="px-3 py-2.5">
              <StatusBadge status={service.status} />
            </td>
            <td className="px-3 py-2.5 text-2xs text-text-secondary">{describeProvenance(service.provenance)}</td>
            <td className="px-3 py-2.5 font-mono text-2xs text-text-secondary">
              <Timestamp value={service.firstObservedAt} />
            </td>
            <td className="px-3 py-2.5 font-mono text-2xs text-text-secondary">
              {service.lastTelemetryAt ? <Timestamp value={service.lastTelemetryAt} /> : '—'}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
