package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/graph"
)

func newTestGraphAPI(t *testing.T) (*GraphAPI, *graph.DependencyGraph) {
	t.Helper()
	depGraph := graph.NewDependencyGraph(300)
	return NewGraphAPI(depGraph), depGraph
}

func TestHandleGetGraph_WrongMethod_Returns405(t *testing.T) {
	api, _ := newTestGraphAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/graph", nil)
	rec := httptest.NewRecorder()
	api.HandleGetGraph(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleGetGraph_ReturnsPopulatedSnapshot(t *testing.T) {
	api, depGraph := newTestGraphAPI(t)
	depGraph.AddDependency("atlas-gateway", "atlas-order-service", 50, false, "OK")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/graph", nil)
	rec := httptest.NewRecorder()
	api.HandleGetGraph(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got correlationmodel.GraphSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got.Edges) != 1 || got.Edges[0].SourceService != "atlas-gateway" || got.Edges[0].TargetService != "atlas-order-service" {
		t.Fatalf("expected 1 edge atlas-gateway->atlas-order-service, got %+v", got.Edges)
	}
}

func TestHandleGetServiceDependencies_Found_ReturnsIncomingAndOutgoing(t *testing.T) {
	api, depGraph := newTestGraphAPI(t)
	depGraph.AddDependency("atlas-gateway", "atlas-order-service", 50, false, "OK")
	depGraph.AddDependency("atlas-order-service", "atlas-payment-service", 30, false, "OK")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/graph/services/atlas-order-service", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceDependencies(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got struct {
		Service  string                             `json:"service"`
		Incoming []*correlationmodel.DependencyEdge `json:"incoming"`
		Outgoing []*correlationmodel.DependencyEdge `json:"outgoing"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got.Incoming) != 1 || len(got.Outgoing) != 1 {
		t.Fatalf("expected 1 incoming and 1 outgoing edge for atlas-order-service, got incoming=%d outgoing=%d", len(got.Incoming), len(got.Outgoing))
	}
}

func TestHandleGetServiceDependencies_UnknownService_Returns404(t *testing.T) {
	api, _ := newTestGraphAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/graph/services/never-observed", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceDependencies(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a service with no observed dependencies, got %d", rec.Code)
	}
}

func TestHandleGetEdges_ReturnsAllObservedEdges(t *testing.T) {
	api, depGraph := newTestGraphAPI(t)
	depGraph.AddDependency("atlas-gateway", "atlas-order-service", 50, false, "OK")
	depGraph.AddDependency("atlas-order-service", "atlas-inventory-service", 20, true, "ERROR")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/graph/edges", nil)
	rec := httptest.NewRecorder()
	api.HandleGetEdges(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []*correlationmodel.DependencyEdge
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(got))
	}
}
