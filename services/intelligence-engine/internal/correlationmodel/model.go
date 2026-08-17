package correlationmodel

import (
	"time"

	"github.com/atlas/intelligence-engine/internal/event"
)

// CorrelatedSpan represents a single reconstructed span in a trace.
type CorrelatedSpan struct {
	SpanID        string            `json:"span_id"`
	ParentSpanID  string            `json:"parent_span_id"`
	TraceID       string            `json:"trace_id"`
	ServiceName   string            `json:"service_name"`
	OperationName string            `json:"operation_name"`
	StartTime     time.Time         `json:"start_time"`
	EndTime       time.Time         `json:"end_time"`
	DurationMs    int64             `json:"duration_ms"`
	Status        string            `json:"status"` // OK, ERROR, UNSET
	Attributes    map[string]string `json:"attributes,omitempty"`
}

// TraceSummary provides a high-level overview of a reconstructed trace.
type TraceSummary struct {
	TraceID                     string   `json:"trace_id"`
	RootService                 string   `json:"root_service"`
	StartTime                   time.Time `json:"start_time"`
	EndTime                     time.Time `json:"end_time"`
	DurationMs                  int64    `json:"duration_ms"`
	SpanCount                   int      `json:"span_count"`
	ServiceCount                int      `json:"service_count"`
	Services                    []string `json:"services"`
	ResolvedRelationships       int      `json:"resolved_relationships"`
	UnresolvedRelationships     int      `json:"unresolved_relationships"`
	OverallStatus               string   `json:"overall_status"`
}

// CorrelatedTrace represents a fully reconstructed trace.
type CorrelatedTrace struct {
	TraceSummary
	Spans []*CorrelatedSpan `json:"spans"`
}

// TreeNode represents a hierarchical node in the trace tree.
type TreeNode struct {
	SpanID        string      `json:"span_id"`
	ParentSpanID  string      `json:"parent_span_id,omitempty"`
	ServiceName   string      `json:"service_name"`
	OperationName string      `json:"operation_name"`
	StartTime     time.Time   `json:"start_time"`
	EndTime       time.Time   `json:"end_time"`
	DurationMs    int64       `json:"duration_ms"`
	Status        string      `json:"status"`
	Children      []*TreeNode `json:"children,omitempty"`
}

// TimelineNode represents a chronological node in the trace timeline.
type TimelineNode struct {
	SpanID        string    `json:"span_id"`
	ServiceName   string    `json:"service_name"`
	OperationName string    `json:"operation_name"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	DurationMs    int64     `json:"duration_ms"`
	Status        string    `json:"status"`
}

// ServiceNode represents a service in the dependency graph.
type ServiceNode struct {
	ServiceName   string    `json:"service_name"`
	FirstObserved time.Time `json:"first_observed"`
	LastObserved  time.Time `json:"last_observed"`
	SpanCount     int64     `json:"span_count"`
}

// DependencyEdge represents an observed directional dependency.
type DependencyEdge struct {
	SourceService     string            `json:"source"`
	TargetService     string            `json:"target"`
	CallCount         int64             `json:"call_count"`
	FirstObserved     time.Time         `json:"first_observed"`
	LastObserved      time.Time         `json:"last_observed"`
	TotalDurationMs   int64             `json:"-"` // Internal use for calculating average
	AverageDurationMs int64             `json:"average_duration_ms"`
	ErrorCount        int64             `json:"error_count"`
	StatusCounts      map[string]int64  `json:"status_counts"`
}

// GraphSnapshot represents a point-in-time snapshot of the dependency graph.
type GraphSnapshot struct {
	Nodes []string          `json:"nodes"`
	Edges []*DependencyEdge `json:"edges"`
}

// FromEvent creates a CorrelatedSpan from an ATLASEvent.
func FromEvent(e *event.ATLASEvent) *CorrelatedSpan {
	endTime := e.Timestamp.Add(time.Duration(e.DurationMs) * time.Millisecond)
	return &CorrelatedSpan{
		SpanID:        e.SpanID,
		ParentSpanID:  e.ParentSpanID,
		TraceID:       e.TraceID,
		ServiceName:   e.ServiceName,
		OperationName: e.OperationName,
		StartTime:     e.Timestamp,
		EndTime:       endTime,
		DurationMs:    e.DurationMs,
		Status:        e.Status,
		Attributes:    e.Attributes, // Can be nil or map
	}
}
