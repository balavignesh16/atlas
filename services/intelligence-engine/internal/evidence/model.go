package evidence

import "time"

type EvidenceType string

const (
	EvidenceTypeErrorRate         EvidenceType = "ERROR_RATE"
	EvidenceTypeLatency           EvidenceType = "LATENCY"
	EvidenceTypeSpanError         EvidenceType = "SPAN_ERROR"
	EvidenceTypeDependencyError   EvidenceType = "DEPENDENCY_ERROR"
	EvidenceTypeDependencyLatency EvidenceType = "DEPENDENCY_LATENCY"
	EvidenceTypeTraceFailure      EvidenceType = "TRACE_FAILURE"
	EvidenceTypeServiceHealth     EvidenceType = "SERVICE_HEALTH"
	EvidenceTypeTemporalSequence  EvidenceType = "TEMPORAL_SEQUENCE"
)

type Evidence struct {
	EvidenceID  string       `json:"evidenceId"`
	Type        EvidenceType `json:"type"`
	Timestamp   time.Time    `json:"timestamp"`
	Service     string       `json:"service"`
	Operation   string       `json:"operation"`
	TraceID     string       `json:"traceId,omitempty"`
	SpanID      string       `json:"spanId,omitempty"`
	Description string       `json:"description"`
	Value       float64      `json:"value"`
	Expected    float64      `json:"expected"`
	Observed    float64      `json:"observed"`
	Source      string       `json:"source"`
}
