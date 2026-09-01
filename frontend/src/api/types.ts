// TypeScript mirrors of the actual Go API models, verified field-by-field
// against services/intelligence-engine/internal/*/model.go. Do not add a
// field here that does not exist in the corresponding Go struct's JSON tag.
//
// Note the real, deliberate inconsistency: Incident/RemediationPlan/
// ExecutionRecord/AnalysisResult serialize camelCase, while ATLASEvent and
// the correlationmodel graph/trace types (DependencyEdge, GraphSnapshot,
// CorrelatedSpan, ...) serialize snake_case. This is not a typo in this
// file -- it reflects two different Go packages with two different JSON
// tag conventions in the actual backend.

// ---- registry (Phase 7B/7C, camelCase) -----------------------------------

export type ServiceProvenance = 'OBSERVED_TELEMETRY' | 'DECLARED' | 'DOCKER' | 'KUBERNETES' | 'CONFIG' | 'INFERRED'
export type ServiceStatus = 'ACTIVE' | 'STALE' | 'RETIRED'
// A fixed reliability class derived from provenance (registry.ConfidenceFor
// on the backend) -- never a fabricated numeric score.
export type ServiceConfidence = 'OBSERVED' | 'DECLARED' | 'INFERRED'

export interface Service {
  name: string
  displayName: string
  provenance: ServiceProvenance
  confidence: ServiceConfidence
  status: ServiceStatus
  firstObservedAt: string
  lastObservedAt: string
  lastTelemetryAt?: string
  createdAt: string
  updatedAt: string
}

// ---- serviceintel (Phase 7D, camelCase) ----------------------------------
// Verified field-by-field against services/intelligence-engine/internal/
// httpapi/intelligence.go's response structs. registry's fields are all
// optional because they are omitted entirely (not zero-valued) when
// registry.known is false -- see intelligence.go's toServiceIntelligenceResponse.

export interface ServiceIntelligenceRegistry {
  known: boolean
  status?: ServiceStatus
  provenance?: ServiceProvenance
  confidence?: ServiceConfidence
  firstObservedAt?: string
  lastObservedAt?: string
}

export interface ServiceIntelligenceDependency {
  service: string
  callCount: number
  errorCount: number
  averageDurationMs: number
  firstObserved: string
  lastObserved: string
}

export interface ServiceIntelligenceDependencies {
  incoming: ServiceIntelligenceDependency[]
  outgoing: ServiceIntelligenceDependency[]
}

export interface ServiceIntelligenceIncident {
  incidentId: string
  status: IncidentStatus
  severity: IncidentSeverity
  title: string
  startedAt: string
  rootService: string
  confidence?: string
}

export interface ServiceIntelligence {
  serviceName: string
  registry: ServiceIntelligenceRegistry
  dependencies: ServiceIntelligenceDependencies
  relevantIncidents: ServiceIntelligenceIncident[]
  generatedAt: string
}

// ---- incidentmodel -----------------------------------------------------

export type IncidentStatus = 'OPEN' | 'ACKNOWLEDGED' | 'RESOLVED'
export type IncidentSeverity = 'INFO' | 'WARNING' | 'CRITICAL'

export interface RootCause {
  service: string
  operation: string
  confidence: string
  score: number
}

export interface Incident {
  incidentId: string
  status: IncidentStatus
  severity: IncidentSeverity
  title: string
  description: string
  startedAt: string
  lastUpdatedAt: string
  resolvedAt?: string
  fingerprint: string
  rootService: string
  rootOperation: string
  affectedServices: string[]
  affectedOperations: string[]
  affectedEdges: string[]
  traceCount: number
  failureCount: number
  traceIds: string[]
  evidenceIds: string[]
  rootCause?: RootCause
  confidence?: string
  score?: number
  detectionReason: string
  correlationGroupId?: string
  primaryIncidentId?: string
  relatedIncidentIds?: string[]
}

// ---- evidence ------------------------------------------------------------

export type EvidenceType =
  | 'ERROR_RATE'
  | 'LATENCY'
  | 'SPAN_ERROR'
  | 'DEPENDENCY_ERROR'
  | 'DEPENDENCY_LATENCY'
  | 'TRACE_FAILURE'
  | 'SERVICE_HEALTH'
  | 'TEMPORAL_SEQUENCE'

export interface Evidence {
  evidenceId: string
  type: EvidenceType
  timestamp: string
  service: string
  operation: string
  traceId?: string
  spanId?: string
  description: string
  value: number
  expected: number
  observed: number
  source: string
}

// ---- aireasoning -----------------------------------------------------

export interface EvidenceReference {
  claim: string
  evidenceIds: string[]
}

export interface AnalysisResult {
  analysisId: string
  incidentId: string
  executiveSummary: string
  incidentStart: string
  incidentDurationMs: number
  observedFacts: EvidenceReference[]
  inferences: EvidenceReference[]
  likelyRootCause: string
  rootCauseConfidence: string
  affectedServices: string[]
  unaffectedServices: string[]
  alternativeExplanations: string[]
  missingEvidence: string[]
  recommendedInvestigations: string[]
  limitations: string
  generatedAt: string
  provider: string
  model: string
}

// ---- remediation -----------------------------------------------------

export type PlanStatus = 'PROPOSED' | 'VALIDATED' | 'APPROVED' | 'REJECTED' | 'EXPIRED'
export type RiskLevel = 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL'

export interface RemediationAction {
  actionId: string
  type: string
  targetService: string
  description: string
  parameters: Record<string, string> | null
  riskLevel: string
  requiresApproval: boolean
  evidenceIds: string[]
  expectedOutcome: string
  verificationRequired: boolean
}

export interface ApprovalMetadata {
  approvedAt?: string
  rejectedAt?: string
  approvalReason?: string
  rejectionReason?: string
  approvedFingerprint?: string
  approvedBy?: string
}

export interface RemediationPlan {
  planId: string
  incidentId: string
  createdAt: string
  status: PlanStatus
  riskLevel: RiskLevel
  confidence: string
  rationale: string
  preconditions: string[]
  actions: RemediationAction[]
  verificationSteps: string[]
  rollbackPlan: string[]
  safetyWarnings: string[]
  evidenceIds: string[]
  requiresApproval: boolean
  planner: string
  plannerVersion: string
  approval: ApprovalMetadata
}

// ---- execution -----------------------------------------------------

export type ExecutionStatus =
  | 'PENDING'
  | 'PRECONDITION_CHECK'
  | 'EXECUTING'
  | 'EXECUTED'
  | 'FAILED'
  | 'DISABLED'
  | 'REJECTED'

export type VerificationStatus =
  | 'PENDING'
  | 'VERIFYING'
  | 'VERIFIED'
  | 'FAILED'
  | 'VERIFICATION_TIMEOUT'
  | 'NOT_REQUIRED'

export interface ExecutionRecord {
  executionId: string
  planId: string
  actionId: string
  incidentId: string
  service: string
  action: string
  evidenceIds: string[]
  approver: string
  approvalFingerprint: string
  startedAt: string
  finishedAt?: string
  executionStatus: ExecutionStatus
  verificationStatus: VerificationStatus
  message?: string
  error?: string
}

// ---- event (snake_case JSON) -----------------------------------------

export type EventType = 'TRACE_SPAN' | 'METRIC'

export interface ATLASEvent {
  event_id: string
  event_type: EventType
  timestamp: string
  service_name: string
  environment?: string
  trace_id?: string
  span_id?: string
  parent_span_id?: string
  operation_name?: string
  status?: string
  status_message?: string
  duration_ms?: number
  metric_name?: string
  metric_description?: string
  metric_type?: string
  value?: number
  unit?: string
  attributes?: Record<string, string>
}

// ---- correlationmodel (snake_case JSON) -------------------------------

export interface DependencyEdge {
  source: string
  target: string
  call_count: number
  first_observed: string
  last_observed: string
  average_duration_ms: number
  error_count: number
  status_counts: Record<string, number>
}

export interface GraphSnapshot {
  nodes: string[]
  edges: DependencyEdge[]
}

// GET /api/v1/graph/services/{name} -- verified against
// httpapi/graph.go's HandleGetServiceDependencies, which wraps the two
// edge lists in an untyped map with these three keys.
export interface ServiceDependencies {
  service: string
  incoming: DependencyEdge[]
  outgoing: DependencyEdge[]
}

export interface CorrelatedSpan {
  span_id: string
  parent_span_id: string
  trace_id: string
  service_name: string
  operation_name: string
  start_time: string
  end_time: string
  duration_ms: number
  status: string
  attributes?: Record<string, string>
}

export interface CorrelatedTrace {
  trace_id: string
  root_service: string
  start_time: string
  end_time: string
  duration_ms: number
  span_count: number
  service_count: number
  services: string[]
  resolved_relationships: number
  unresolved_relationships: number
  overall_status: string
  spans: CorrelatedSpan[]
}
