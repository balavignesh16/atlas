import { useNavigate } from '@tanstack/react-router'
import { useRef } from 'react'
import type { ExecutionRecord } from '@/api/types'
import { StatusBadge } from '@/components/status/StatusBadge'
import { IdChip } from '@/components/ui/IdChip'
import { Timestamp } from '@/components/ui/Timestamp'

/**
 * Dense table, execution and verification always shown as two separate
 * columns/badges -- never collapsed into one "result" column. Row click and
 * Enter navigate to /executions/$executionId (a real, deep-linkable route
 * that opens the record as a slide-over -- see ExecutionsPage).
 */
export function ExecutionTable({ records }: { records: ExecutionRecord[] }) {
  const navigate = useNavigate()
  const rowRefs = useRef<Array<HTMLTableRowElement | null>>([])

  function open(executionId: string) {
    navigate({ to: '/executions/$executionId', params: { executionId } })
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLTableRowElement>, index: number, executionId: string) {
    if (e.key === 'Enter') {
      open(executionId)
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
          <th className="px-3 py-2 font-medium">Execution</th>
          <th className="px-3 py-2 font-medium">Incident</th>
          <th className="px-3 py-2 font-medium">Service</th>
          <th className="px-3 py-2 font-medium">Action</th>
          <th className="w-32 px-3 py-2 font-medium">Execution</th>
          <th className="w-40 px-3 py-2 font-medium">Verification</th>
          <th className="w-24 px-3 py-2 font-medium">Started</th>
        </tr>
      </thead>
      <tbody>
        {records.map((record, index) => (
          <tr
            key={record.executionId}
            ref={(el) => {
              rowRefs.current[index] = el
            }}
            tabIndex={0}
            role="row"
            aria-label={`Execution ${record.action} on ${record.service}`}
            onClick={() => open(record.executionId)}
            onKeyDown={(e) => onKeyDown(e, index, record.executionId)}
            className="cursor-pointer border-b border-border-subtle text-xs text-text-primary outline-none transition-colors hover:bg-surface-2 focus-visible:bg-surface-2"
          >
            <td className="px-3 py-2.5">
              <IdChip value={record.executionId} truncate={6} />
            </td>
            <td className="px-3 py-2.5">
              <IdChip value={record.incidentId} truncate={6} />
            </td>
            <td className="px-3 py-2.5 font-medium">{record.service}</td>
            <td className="px-3 py-2.5 font-mono text-2xs text-text-secondary">{record.action}</td>
            <td className="px-3 py-2.5">
              <StatusBadge status={record.executionStatus} />
            </td>
            <td className="px-3 py-2.5">
              <StatusBadge status={record.verificationStatus} />
            </td>
            <td className="px-3 py-2.5 font-mono text-2xs text-text-secondary">
              <Timestamp value={record.startedAt} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
