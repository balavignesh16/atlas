package incidentsignal

import (
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
)

type SignalType string

const (
	SignalTypeErrorRate         SignalType = "ERROR_RATE"
	SignalTypeLatency           SignalType = "LATENCY"
	SignalTypeDependencyFailure SignalType = "DEPENDENCY_FAILURE"
	SignalTypeTraceFailure      SignalType = "TRACE_FAILURE"
)

type Signal struct {
	SignalID  string            `json:"signalId"`
	Type      SignalType        `json:"type"`
	Timestamp time.Time         `json:"timestamp"`
	Service   string            `json:"service"`
	Operation string            `json:"operation"`
	Value     float64           `json:"value"`
	Threshold float64           `json:"threshold"`
	Direction string            `json:"direction"`
	TraceID   string            `json:"traceId,omitempty"`
	SpanID    string            `json:"spanId,omitempty"`
	Evidence  evidence.Evidence `json:"evidence"`
}
