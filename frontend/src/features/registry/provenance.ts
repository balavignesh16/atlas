import type { ServiceConfidence, ServiceProvenance } from '@/api/types'

/**
 * Human-readable labels so a user never needs to know the internal enum
 * names. Only OBSERVED_TELEMETRY is ever actually produced by the backend
 * today (see docs/registry.md) -- the other five labels exist so the UI
 * doesn't need updating the day a second source is implemented, but they
 * are not claims that those sources exist now.
 */
const PROVENANCE_LABELS: Record<ServiceProvenance, string> = {
  OBSERVED_TELEMETRY: 'Observed via telemetry',
  DOCKER: 'Observed via Docker',
  KUBERNETES: 'Observed via Kubernetes',
  DECLARED: 'Declared in configuration',
  CONFIG: 'Inferred from a configuration reference',
  INFERRED: 'Inferred (lowest confidence)',
}

export function describeProvenance(provenance: ServiceProvenance): string {
  return PROVENANCE_LABELS[provenance] ?? provenance
}

const CONFIDENCE_LABELS: Record<ServiceConfidence, string> = {
  OBSERVED: 'Directly observed',
  DECLARED: 'Declared, not directly observed',
  INFERRED: 'Inferred (lowest confidence)',
}

export function describeConfidence(confidence: ServiceConfidence): string {
  return CONFIDENCE_LABELS[confidence] ?? confidence
}
