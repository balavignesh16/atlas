// Derives a single, honest display state for an incident by combining its
// own Status with any execution records fetched for it. Every branch below
// maps to a real backend enum value (IncidentStatus, ExecutionStatus, or
// VerificationStatus) -- nothing here is invented. When no execution data
// is available (not fetched, or the principal lacks READ_AUDIT), this
// falls back to the incident's own Status, which is always safe to show.

import type { ExecutionRecord, Incident } from '@/api/types'

export type IncidentDisplayState =
  | 'OPEN'
  | 'ACKNOWLEDGED'
  | 'RESOLVED'
  | 'EXECUTING'
  | 'VERIFYING'
  | 'VERIFIED'
  | 'VERIFICATION_FAILED'
  | 'VERIFICATION_TIMEOUT'
  | 'EXECUTION_FAILED'
  | 'EXECUTION_REJECTED'

export function deriveIncidentState(
  incident: Incident,
  executions?: ExecutionRecord[],
): IncidentDisplayState {
  if (incident.status === 'RESOLVED') return 'RESOLVED'

  if (executions && executions.length > 0) {
    if (executions.some((e) => e.executionStatus === 'EXECUTING' || e.executionStatus === 'PRECONDITION_CHECK')) {
      return 'EXECUTING'
    }

    const latest = [...executions].sort(
      (a, b) => new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime(),
    )[0]

    if (latest.executionStatus === 'EXECUTED') {
      switch (latest.verificationStatus) {
        case 'VERIFYING':
          return 'VERIFYING'
        case 'VERIFIED':
          return 'VERIFIED'
        case 'FAILED':
          return 'VERIFICATION_FAILED'
        case 'VERIFICATION_TIMEOUT':
          return 'VERIFICATION_TIMEOUT'
        default:
          break
      }
    }
    if (latest.executionStatus === 'FAILED') return 'EXECUTION_FAILED'
    if (latest.executionStatus === 'REJECTED') return 'EXECUTION_REJECTED'
  }

  return incident.status
}
