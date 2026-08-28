package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
)

func newTestCorrelationAPI(t *testing.T) (*CorrelationAPI, *correlation.Engine) {
	t.Helper()
	depGraph := graph.NewDependencyGraph(300)
	corrEngine := correlation.NewEngine(depGraph, 300)
	return NewCorrelationAPI(corrEngine), corrEngine
}

// seedTrace feeds a real parent+child span pair through the real
// correlation.Engine so the handler under test operates on genuinely
// reconstructed trace data, matching this project's no-mocks convention.
func seedTrace(corrEngine *correlation.Engine, traceID string) {
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID:     "parent-ev",
		EventType:   event.EventTypeTraceSpan,
		TraceID:     traceID,
		SpanID:      "span-parent",
		ServiceName: "atlas-gateway",
		Status:      "OK",
	})
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID:      "child-ev",
		EventType:    event.EventTypeTraceSpan,
		TraceID:      traceID,
		SpanID:       "span-child",
		ParentSpanID: "span-parent",
		ServiceName:  "atlas-order-service",
		Status:       "OK",
	})
}

func TestHandleGetTrace_WrongMethod_Returns405(t *testing.T) {
	api, _ := newTestCorrelationAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/correlations/traces/trace-1", nil)
	rec := httptest.NewRecorder()
	api.HandleGetTrace(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleGetTrace_NotFound_Returns404(t *testing.T) {
	api, _ := newTestCorrelationAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/correlations/traces/does-not-exist", nil)
	rec := httptest.NewRecorder()
	api.HandleGetTrace(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown traceId, got %d", rec.Code)
	}
}

func TestHandleGetTrace_Found_ReturnsReconstructedTrace(t *testing.T) {
	api, corrEngine := newTestCorrelationAPI(t)
	seedTrace(corrEngine, "trace-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/correlations/traces/trace-1", nil)
	rec := httptest.NewRecorder()
	api.HandleGetTrace(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got correlationmodel.CorrelatedTrace
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if got.TraceID != "trace-1" || got.SpanCount != 2 {
		t.Fatalf("expected trace-1 with 2 spans, got TraceID=%q SpanCount=%d", got.TraceID, got.SpanCount)
	}
}

func TestHandleGetTraceTree_Found_ReturnsHierarchy(t *testing.T) {
	api, corrEngine := newTestCorrelationAPI(t)
	seedTrace(corrEngine, "trace-2")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/correlations/traces/trace-2/tree", nil)
	rec := httptest.NewRecorder()
	api.HandleGetTraceTree(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []*correlationmodel.TreeNode
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 1 || got[0].ServiceName != "atlas-gateway" || len(got[0].Children) != 1 {
		t.Fatalf("expected a single root (atlas-gateway) with 1 child, got %+v", got)
	}
}

func TestHandleGetTraceTree_NotFound_Returns404(t *testing.T) {
	api, _ := newTestCorrelationAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/correlations/traces/does-not-exist/tree", nil)
	rec := httptest.NewRecorder()
	api.HandleGetTraceTree(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown traceId, got %d", rec.Code)
	}
}

func TestHandleGetTraceTimeline_Found_ReturnsChronologicalEntries(t *testing.T) {
	api, corrEngine := newTestCorrelationAPI(t)
	seedTrace(corrEngine, "trace-3")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/correlations/traces/trace-3/timeline", nil)
	rec := httptest.NewRecorder()
	api.HandleGetTraceTimeline(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []*correlationmodel.TimelineNode
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 timeline entries, got %d", len(got))
	}
}
