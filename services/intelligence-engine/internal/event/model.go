package event

import "time"

// Event types
const (
	EventTypeTraceSpan = "TRACE_SPAN"
	EventTypeMetric    = "METRIC"
)

// ATLASEvent is the canonical normalized event model for ATLAS.
type ATLASEvent struct {
	EventID       string            `json:"event_id"`
	EventType     string            `json:"event_type"` // TRACE_SPAN, METRIC
	Timestamp     time.Time         `json:"timestamp"`
	ServiceName   string            `json:"service_name"`
	Environment   string            `json:"environment,omitempty"`
	TraceID       string            `json:"trace_id,omitempty"`
	SpanID        string            `json:"span_id,omitempty"`
	ParentSpanID  string            `json:"parent_span_id,omitempty"`
	OperationName string            `json:"operation_name,omitempty"`
	Status        string            `json:"status,omitempty"` // OK, ERROR, UNSET
	StatusMessage string            `json:"status_message,omitempty"`
	DurationMs    int64             `json:"duration_ms,omitempty"`
	MetricName    string            `json:"metric_name,omitempty"`
	MetricDesc    string            `json:"metric_description,omitempty"`
	MetricType    string            `json:"metric_type,omitempty"`
	Value         float64           `json:"value,omitempty"`
	Unit          string            `json:"unit,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"`
}

// IsErrorStatus classifies a span/event as an error using the OTel span
// status first, falling back to string status-code attributes for
// instrumentation stacks that don't set span status on a clean 5xx response
// (this project's actual Micrometer/Spring telemetry never marks a
// server-side span's own Status as ERROR for a handled 500 -- the real code
// only appears as a string attribute). Shared by every consumer that needs
// to know whether a span/event represents a failure, so this classification
// is defined in exactly one place. Semantics are unchanged from the
// original incidentdetector-only implementation: ERROR/5xx status, or a
// leading '5' in http.response.status_code, http.status_code, or status.
func IsErrorStatus(status string, attributes map[string]string) bool {
	if status == "ERROR" || status == "5xx" {
		return true
	}
	for _, key := range []string{"http.response.status_code", "http.status_code", "status"} {
		if v, ok := attributes[key]; ok && len(v) > 0 && v[0] == '5' {
			return true
		}
	}
	return false
}
