import type { ExecutionRecord } from '@/api/types'

/** Client-side filter over already-fetched execution records only -- there
 * is no backend search endpoint. Matches real fields: execution ID,
 * incident ID, service, and action. */
export function filterExecutions(records: ExecutionRecord[], query: string): ExecutionRecord[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return records
  return records.filter(
    (r) =>
      r.executionId.toLowerCase().includes(needle) ||
      r.incidentId.toLowerCase().includes(needle) ||
      r.service.toLowerCase().includes(needle) ||
      r.action.toLowerCase().includes(needle),
  )
}

export function sortExecutionsByStartedAt(records: ExecutionRecord[]): ExecutionRecord[] {
  return [...records].sort((a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime())
}
