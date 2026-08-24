package incidentdetector

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
)

func newTestDetector() *Detector {
	signals := make(chan incidentsignal.Signal, 10)
	g := graph.NewDependencyGraph(3600)
	return NewDetector(DefaultConfig(), g, signals)
}

// Regression test for the bug where ProcessEvent compared e.EventType against
// the literal string "SPAN" instead of event.EventTypeTraceSpan ("TRACE_SPAN").
// Since every normalized trace event carries EventTypeTraceSpan, that comparison
// was always true and every event was silently dropped, making the sliding-window
// detector a no-op in the live pipeline from M2.4's original commit onward.
func TestProcessEvent_TraceSpanIsRecordedInWindow(t *testing.T) {
	d := newTestDetector()

	ev := event.ATLASEvent{
		EventType:     event.EventTypeTraceSpan,
		ServiceName:   "atlas-payment-service",
		OperationName: "POST /api/payments",
		Status:        "ERROR",
		DurationMs:    120,
		Timestamp:     time.Now(),
	}

	d.ProcessEvent(ev)

	w := d.getWindow(ev.ServiceName, ev.OperationName)
	if got := w.Count(); got != 1 {
		t.Fatalf("expected 1 observation recorded after processing a TRACE_SPAN event, got %d", got)
	}
	if got := w.ErrorRate(); got != 1.0 {
		t.Fatalf("expected error rate 1.0 for a single ERROR observation, got %f", got)
	}
}

func TestProcessEvent_NonTraceSpanEventsAreIgnored(t *testing.T) {
	d := newTestDetector()

	ev := event.ATLASEvent{
		EventType:   event.EventTypeMetric,
		ServiceName: "atlas-payment-service",
		Timestamp:   time.Now(),
	}

	d.ProcessEvent(ev)

	w := d.getWindow(ev.ServiceName, ev.OperationName)
	if got := w.Count(); got != 0 {
		t.Fatalf("expected METRIC events to be ignored by the incident detector, got %d observations", got)
	}
}

// HTTP status codes carried as attributes (e.g. from server spans) should also
// mark an observation as an error even when Status itself is not "ERROR"/"5xx".
func TestProcessEvent_HttpStatusCodeAttributeMarksError(t *testing.T) {
	d := newTestDetector()

	ev := event.ATLASEvent{
		EventType:     event.EventTypeTraceSpan,
		ServiceName:   "atlas-order-service",
		OperationName: "POST /api/orders",
		Status:        "OK",
		Attributes:    map[string]string{"http.response.status_code": "500"},
		Timestamp:     time.Now(),
	}

	d.ProcessEvent(ev)

	w := d.getWindow(ev.ServiceName, ev.OperationName)
	if got := w.ErrorRate(); got != 1.0 {
		t.Fatalf("expected a 5xx status_code attribute to be treated as an error, got error rate %f", got)
	}
}

// Regression test (M2.7.1): a live docker-compose run showed that this
// project's actual Micrometer/Spring instrumentation never sets the span's
// own Status field to ERROR for a server-side span, even on a genuine 500
// response -- it always stays "UNSET" -- and carries the real HTTP status
// under the plain attribute key "status" (e.g. {"status": "500", "outcome":
// "SERVER_ERROR"}), not "http.status_code"/"http.response.status_code".
// Before this fix, that meant a service with no outbound calls of its own
// (e.g. atlas-payment-service, a pure sink) could never register an error at
// all, since it has no client-side span to fall back on either -- making it
// impossible for such a service to ever become an incident's root cause.
func TestProcessEvent_MicrometerStatusAttributeMarksError(t *testing.T) {
	d := newTestDetector()

	ev := event.ATLASEvent{
		EventType:     event.EventTypeTraceSpan,
		ServiceName:   "atlas-payment-service",
		OperationName: "http post /api/payments",
		Status:        "UNSET",
		Attributes:    map[string]string{"status": "500", "outcome": "SERVER_ERROR"},
		Timestamp:     time.Now(),
	}

	d.ProcessEvent(ev)

	w := d.getWindow(ev.ServiceName, ev.OperationName)
	if got := w.ErrorRate(); got != 1.0 {
		t.Fatalf("expected the Micrometer-style \"status\" attribute to be treated as an error, got error rate %f", got)
	}
}

func TestProcessEvent_MicrometerStatusAttributeSuccessIsNotAnError(t *testing.T) {
	d := newTestDetector()

	ev := event.ATLASEvent{
		EventType:     event.EventTypeTraceSpan,
		ServiceName:   "atlas-payment-service",
		OperationName: "http post /api/payments",
		Status:        "UNSET",
		Attributes:    map[string]string{"status": "201", "outcome": "SUCCESS"},
		Timestamp:     time.Now(),
	}

	d.ProcessEvent(ev)

	w := d.getWindow(ev.ServiceName, ev.OperationName)
	if got := w.ErrorRate(); got != 0.0 {
		t.Fatalf("expected a 2xx \"status\" attribute to NOT be treated as an error, got error rate %f", got)
	}
}
