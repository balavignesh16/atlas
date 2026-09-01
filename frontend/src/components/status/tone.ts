// Single mapping from every real backend status/severity value this app
// displays to one of the design system's semantic status tones. Adding a
// new displayed status means adding one line here -- never picking a raw
// color in a component.

export type StatusTone =
  | 'healthy'
  | 'warning'
  | 'critical'
  | 'executing'
  | 'failed'
  | 'pending'
  | 'timeout'

const TONE_MAP: Record<string, StatusTone> = {
  // IncidentSeverity
  INFO: 'pending',
  WARNING: 'warning',
  CRITICAL: 'critical',

  // IncidentStatus / IncidentDisplayState
  OPEN: 'warning',
  ACKNOWLEDGED: 'pending',
  RESOLVED: 'healthy',

  // ExecutionStatus
  PENDING: 'pending',
  PRECONDITION_CHECK: 'pending',
  EXECUTING: 'executing',
  EXECUTED: 'healthy',
  FAILED: 'failed',
  EXECUTION_FAILED: 'failed',
  EXECUTION_REJECTED: 'failed',
  DISABLED: 'pending',
  REJECTED: 'failed',

  // VerificationStatus / derived display states
  VERIFYING: 'executing',
  VERIFIED: 'healthy',
  VERIFICATION_FAILED: 'failed',
  VERIFICATION_TIMEOUT: 'timeout',
  NOT_REQUIRED: 'pending',

  // PlanStatus
  PROPOSED: 'pending',
  VALIDATED: 'pending',
  APPROVED: 'healthy',
  EXPIRED: 'timeout',

  // Registry Status (Phase 7B) -- a service's canonical lifecycle state,
  // independent of the telemetry graph's own retention/expiry.
  ACTIVE: 'healthy',
  STALE: 'warning',
  RETIRED: 'pending',
}

export function toneFor(status: string): StatusTone {
  return TONE_MAP[status] ?? 'pending'
}

export const TONE_CLASSES: Record<StatusTone, { text: string; bg: string; dot: string }> = {
  healthy: { text: 'text-status-healthy', bg: 'bg-status-healthy-bg', dot: 'bg-status-healthy' },
  warning: { text: 'text-status-warning', bg: 'bg-status-warning-bg', dot: 'bg-status-warning' },
  critical: { text: 'text-status-critical', bg: 'bg-status-critical-bg', dot: 'bg-status-critical' },
  executing: { text: 'text-status-executing', bg: 'bg-status-executing-bg', dot: 'bg-status-executing' },
  failed: { text: 'text-status-failed', bg: 'bg-status-failed-bg', dot: 'bg-status-failed' },
  pending: { text: 'text-status-pending', bg: 'bg-status-pending-bg', dot: 'bg-status-pending' },
  timeout: { text: 'text-status-timeout', bg: 'bg-status-timeout-bg', dot: 'bg-status-timeout' },
}
