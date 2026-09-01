package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/registry"
	"github.com/atlas/intelligence-engine/internal/serviceintel"
)

func newTestIntelligenceAPI(t *testing.T) (*IntelligenceAPI, *registry.Store, *graph.DependencyGraph, *incidentmanager.Manager) {
	t.Helper()
	store, err := registry.NewStore(":memory:")
	if err != nil {
		t.Fatalf("registry.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	depGraph := graph.NewDependencyGraph(300)
	incManager := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evidence.NewStore())
	assembler := serviceintel.NewAssembler(store, depGraph, incManager)
	return NewIntelligenceAPI(assembler), store, depGraph, incManager
}

func newIntelTestSignal(service, operation string, at time.Time) incidentsignal.Signal {
	return incidentsignal.Signal{
		SignalID:  "sig-" + service + "-" + operation,
		Type:      incidentsignal.SignalTypeErrorRate,
		Timestamp: at,
		Service:   service,
		Operation: operation,
		Value:     0.5,
		Threshold: 0.2,
		Evidence: evidence.Evidence{
			EvidenceID:  "ev-" + service + "-" + operation,
			Type:        evidence.EvidenceTypeErrorRate,
			Timestamp:   at,
			Service:     service,
			Description: "test",
		},
	}
}

func TestHandleGetServiceIntelligence_WrongMethod_Returns405(t *testing.T) {
	api, _, _, _ := newTestIntelligenceAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/checkout-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "checkout-service")

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleGetServiceIntelligence_TotallyUnknown_Returns404(t *testing.T) {
	api, _, _, _ := newTestIntelligenceAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/never-heard-of-it/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "never-heard-of-it")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a name unknown to all sources, got %d", rec.Code)
	}
}

func TestHandleGetServiceIntelligence_MissingName_Returns400(t *testing.T) {
	api, _, _, _ := newTestIntelligenceAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services//intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a missing service name, got %d", rec.Code)
	}
}

func TestHandleGetServiceIntelligence_RegistryOnly_Returns200(t *testing.T) {
	api, store, _, _ := newTestIntelligenceAPI(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("checkout-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/checkout-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "checkout-service")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got["serviceName"] != "checkout-service" {
		t.Errorf("serviceName = %v, want checkout-service", got["serviceName"])
	}
	reg, ok := got["registry"].(map[string]any)
	if !ok {
		t.Fatalf("expected a registry object, got %T", got["registry"])
	}
	if reg["known"] != true {
		t.Errorf("registry.known = %v, want true", reg["known"])
	}
	if reg["status"] != "ACTIVE" {
		t.Errorf("registry.status = %v, want ACTIVE", reg["status"])
	}
}

func TestHandleGetServiceIntelligence_UnknownToRegistry_OmitsRegistryFields(t *testing.T) {
	api, _, depGraph, _ := newTestIntelligenceAPI(t)
	depGraph.AddDependency("gateway", "graph-only-service", 15, false, "OK")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/graph-only-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "graph-only-service")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	reg, ok := got["registry"].(map[string]any)
	if !ok {
		t.Fatalf("expected a registry object, got %T", got["registry"])
	}
	if reg["known"] != false {
		t.Errorf("registry.known = %v, want false", reg["known"])
	}
	for _, field := range []string{"status", "provenance", "confidence", "firstObservedAt", "lastObservedAt"} {
		if _, present := reg[field]; present {
			t.Errorf("expected registry.%s to be entirely absent when known=false, got %v", field, reg[field])
		}
	}
}

func TestHandleGetServiceIntelligence_DependencyFields(t *testing.T) {
	api, _, depGraph, _ := newTestIntelligenceAPI(t)
	depGraph.AddDependency("gateway", "checkout-service", 20, false, "OK")
	depGraph.AddDependency("checkout-service", "payment-service", 30, true, "ERROR")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/checkout-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "checkout-service")

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	deps, ok := got["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("expected a dependencies object, got %T", got["dependencies"])
	}
	incoming, ok := deps["incoming"].([]any)
	if !ok || len(incoming) != 1 {
		t.Fatalf("expected 1 incoming dependency, got %+v", deps["incoming"])
	}
	first := incoming[0].(map[string]any)
	if first["service"] != "gateway" {
		t.Errorf("incoming[0].service = %v, want gateway", first["service"])
	}
	outgoing, ok := deps["outgoing"].([]any)
	if !ok || len(outgoing) != 1 {
		t.Fatalf("expected 1 outgoing dependency, got %+v", deps["outgoing"])
	}
}

func TestHandleGetServiceIntelligence_IncidentAssociation(t *testing.T) {
	api, _, _, incManager := newTestIntelligenceAPI(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	incManager.ProcessSignal(newIntelTestSignal("checkout-service", "checkout", at))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/checkout-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "checkout-service")

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	incidents, ok := got["relevantIncidents"].([]any)
	if !ok || len(incidents) != 1 {
		t.Fatalf("expected 1 relevant incident, got %+v", got["relevantIncidents"])
	}
	inc := incidents[0].(map[string]any)
	if inc["rootService"] != "checkout-service" {
		t.Errorf("relevantIncidents[0].rootService = %v, want checkout-service", inc["rootService"])
	}
}

func TestHandleGetServiceIntelligence_IncidentOnly_Returns200(t *testing.T) {
	api, _, _, incManager := newTestIntelligenceAPI(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	incManager.ProcessSignal(newIntelTestSignal("incident-only-service", "op", at))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/incident-only-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "incident-only-service")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a service known only through incident history, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	reg := got["registry"].(map[string]any)
	if reg["known"] != false {
		t.Errorf("registry.known = %v, want false", reg["known"])
	}
	incidents := got["relevantIncidents"].([]any)
	if len(incidents) != 1 {
		t.Fatalf("expected 1 relevant incident, got %d", len(incidents))
	}
}

func TestHandleGetServiceIntelligence_EmptyCollections_ReturnEmptyArraysNotNull(t *testing.T) {
	api, store, _, _ := newTestIntelligenceAPI(t)
	if err := store.Observe("lonely-service", time.Now()); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/lonely-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "lonely-service")

	body := rec.Body.String()
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	deps := got["dependencies"].(map[string]any)
	if _, ok := deps["incoming"].([]any); !ok {
		t.Errorf("expected dependencies.incoming to decode as an array, got %T in body %s", deps["incoming"], body)
	}
	if _, ok := deps["outgoing"].([]any); !ok {
		t.Errorf("expected dependencies.outgoing to decode as an array, got %T", deps["outgoing"])
	}
	if _, ok := got["relevantIncidents"].([]any); !ok {
		t.Errorf("expected relevantIncidents to decode as an array, got %T", got["relevantIncidents"])
	}
}

func TestHandleGetServiceIntelligence_NoUndocumentedFields(t *testing.T) {
	api, store, depGraph, incManager := newTestIntelligenceAPI(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("checkout-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	depGraph.AddDependency("gateway", "checkout-service", 20, false, "OK")
	incManager.ProcessSignal(newIntelTestSignal("checkout-service", "checkout", at))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/checkout-service/intelligence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetServiceIntelligence(rec, req, "checkout-service")

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	allowedTop := map[string]bool{
		"serviceName": true, "registry": true, "dependencies": true,
		"relevantIncidents": true, "generatedAt": true,
	}
	for key := range got {
		if !allowedTop[key] {
			t.Errorf("unexpected top-level field %q -- no field beyond the documented contract should be exposed", key)
		}
	}

	allowedRegistry := map[string]bool{
		"known": true, "status": true, "provenance": true, "confidence": true,
		"firstObservedAt": true, "lastObservedAt": true,
	}
	for key := range got["registry"].(map[string]any) {
		if !allowedRegistry[key] {
			t.Errorf("unexpected registry field %q", key)
		}
	}

	deps := got["dependencies"].(map[string]any)
	incoming := deps["incoming"].([]any)
	if len(incoming) != 1 {
		t.Fatalf("expected 1 incoming dependency, got %d", len(incoming))
	}
	allowedDependency := map[string]bool{
		"service": true, "callCount": true, "errorCount": true,
		"averageDurationMs": true, "firstObserved": true, "lastObserved": true,
	}
	for key := range incoming[0].(map[string]any) {
		if !allowedDependency[key] {
			t.Errorf("unexpected dependency field %q", key)
		}
	}

	incidents := got["relevantIncidents"].([]any)
	if len(incidents) != 1 {
		t.Fatalf("expected 1 relevant incident, got %d", len(incidents))
	}
	allowedIncident := map[string]bool{
		"incidentId": true, "status": true, "severity": true, "title": true,
		"startedAt": true, "rootService": true, "confidence": true,
	}
	for key := range incidents[0].(map[string]any) {
		if !allowedIncident[key] {
			t.Errorf("unexpected incident field %q", key)
		}
	}
}
