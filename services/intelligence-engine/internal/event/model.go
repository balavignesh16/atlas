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
