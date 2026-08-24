package correlationmodel

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/event"
)

func TestFromEvent_DerivesStatusViaSharedClassifier(t *testing.T) {
	ev := &event.ATLASEvent{
		SpanID:      "span-1",
		TraceID:     "trace-1",
		ServiceName: "atlas-payment-service",
		Timestamp:   time.Now(),
		Status:      "UNSET",
		Attributes:  map[string]string{"status": "500"},
	}

	span := FromEvent(ev)

	if span.Status != "ERROR" {
		t.Fatalf("expected a real 5xx attribute to normalize Status to ERROR, got %q", span.Status)
	}
}

func TestFromEvent_NonErrorStatusPassesThroughUnchanged(t *testing.T) {
	cases := []struct {
		name   string
		status string
		attrs  map[string]string
	}{
		{"OK with 2xx attribute", "OK", map[string]string{"status": "201"}},
		{"UNSET with no attributes", "UNSET", nil},
		{"UNSET with 4xx attribute (not an error)", "UNSET", map[string]string{"status": "409"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev := &event.ATLASEvent{
				SpanID:      "span-1",
				TraceID:     "trace-1",
				ServiceName: "atlas-order-service",
				Timestamp:   time.Now(),
				Status:      c.status,
				Attributes:  c.attrs,
			}
			span := FromEvent(ev)
			if span.Status != c.status {
				t.Fatalf("expected non-error Status %q to pass through unchanged, got %q", c.status, span.Status)
			}
		})
	}
}

func TestFromEvent_ExplicitErrorStatusStillNormalizesToERROR(t *testing.T) {
	ev := &event.ATLASEvent{
		SpanID:      "span-1",
		TraceID:     "trace-1",
		ServiceName: "atlas-order-service",
		Timestamp:   time.Now(),
		Status:      "5xx",
	}
	span := FromEvent(ev)
	if span.Status != "ERROR" {
		t.Fatalf("expected Status=5xx to normalize to ERROR, got %q", span.Status)
	}
}
