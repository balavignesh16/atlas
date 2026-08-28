package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/event"
)

func newTestVerificationAPI(t *testing.T) (*VerificationAPI, *buffer.EventBuffer) {
	t.Helper()
	buf := buffer.NewEventBuffer(100)
	return NewVerificationAPI(buf), buf
}

func TestHandleGetEvents_ReturnsAllBufferedEvents(t *testing.T) {
	api, buf := newTestVerificationAPI(t)
	buf.Add(event.ATLASEvent{EventID: "ev-1", EventType: event.EventTypeTraceSpan, ServiceName: "atlas-payment-service"})
	buf.Add(event.ATLASEvent{EventID: "ev-2", EventType: event.EventTypeMetric, ServiceName: "atlas-order-service"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	rec := httptest.NewRecorder()
	api.HandleGetEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []event.ATLASEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
}

func TestHandleGetEventByID_Found_ReturnsMatchingEvent(t *testing.T) {
	api, buf := newTestVerificationAPI(t)
	buf.Add(event.ATLASEvent{EventID: "ev-target", ServiceName: "atlas-inventory-service"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/ev-target", nil)
	rec := httptest.NewRecorder()
	api.HandleGetEventByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got event.ATLASEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if got.EventID != "ev-target" {
		t.Errorf("expected EventID ev-target, got %q", got.EventID)
	}
}

func TestHandleGetEventByID_NotFound_Returns404(t *testing.T) {
	api, _ := newTestVerificationAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/does-not-exist", nil)
	rec := httptest.NewRecorder()
	api.HandleGetEventByID(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a missing event ID, got %d", rec.Code)
	}
}

func TestHandleGetEventsByTrace_FiltersToMatchingTraceSpansOnly(t *testing.T) {
	api, buf := newTestVerificationAPI(t)
	now := time.Now()
	buf.Add(event.ATLASEvent{EventID: "span-1", EventType: event.EventTypeTraceSpan, TraceID: "trace-A", Timestamp: now})
	buf.Add(event.ATLASEvent{EventID: "span-2", EventType: event.EventTypeTraceSpan, TraceID: "trace-B", Timestamp: now})
	buf.Add(event.ATLASEvent{EventID: "metric-1", EventType: event.EventTypeMetric, TraceID: "trace-A", Timestamp: now})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/trace/trace-A", nil)
	rec := httptest.NewRecorder()
	api.HandleGetEventsByTrace(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []event.ATLASEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 1 || got[0].EventID != "span-1" {
		t.Fatalf("expected only the trace-A span, got %+v", got)
	}
}

func TestHandleGetMetrics_FiltersToMetricEventsOnly(t *testing.T) {
	api, buf := newTestVerificationAPI(t)
	buf.Add(event.ATLASEvent{EventID: "span-1", EventType: event.EventTypeTraceSpan})
	buf.Add(event.ATLASEvent{EventID: "metric-1", EventType: event.EventTypeMetric, MetricName: "request.latency"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/metrics", nil)
	rec := httptest.NewRecorder()
	api.HandleGetMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []event.ATLASEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 1 || got[0].EventID != "metric-1" {
		t.Fatalf("expected only the metric event, got %+v", got)
	}
}
