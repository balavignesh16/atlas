package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	aiprovider "github.com/atlas/intelligence-engine/internal/aireasoning/provider"
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/propagation"
	"github.com/atlas/intelligence-engine/internal/rca"
)

// newTestIncidentAPI wires a real, unmocked IncidentAPI against real
// dependencies, matching this project's established testing convention.
func newTestIncidentAPI(t *testing.T) (*IncidentAPI, *incidentmanager.Manager) {
	t.Helper()
	evStore := evidence.NewStore()
	depGraph := graph.NewDependencyGraph(300)
	corrEngine := correlation.NewEngine(depGraph, 300)
	manager := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evStore)
	propAnalyzer := propagation.NewAnalyzer(depGraph, corrEngine)
	rcaEngine := rca.NewEngine(evStore, propAnalyzer, depGraph)
	aiCfg := aireasoning.Config{
		Enabled:          true,
		MaxEvents:        200,
		MaxSpans:         200,
		MaxServices:      50,
		MaxAttributes:    50,
		MaxStringLength:  1024,
		TimeoutSeconds:   30,
		RetentionSeconds: 3600,
	}
	aiEngine := aireasoning.NewEngine(aiCfg, aiprovider.NewFakeProvider())

	api := NewIncidentAPI(manager, evStore, rcaEngine, corrEngine, aiEngine, depGraph)
	return api, manager
}

// seedIncident creates a real incident via Manager.ProcessSignal (the actual
// production creation path) and returns its generated IncidentID.
func seedIncident(manager *incidentmanager.Manager, service string) string {
	manager.ProcessSignal(incidentsignal.Signal{
		SignalID:  "sig-1",
		Type:      incidentsignal.SignalTypeErrorRate,
		Timestamp: time.Now(),
		Service:   service,
		Operation: "http post /api/orders",
		Value:     0.9,
		Threshold: 0.5,
		Evidence: evidence.Evidence{
			EvidenceID:  "ev-seed-1",
			Type:        evidence.EvidenceTypeErrorRate,
			Service:     service,
			Description: "error rate exceeded threshold",
		},
	})
	open := manager.GetOpenIncidents()
	if len(open) != 1 {
		panic("seedIncident: expected exactly 1 open incident after ProcessSignal")
	}
	return open[0].IncidentID
}

func TestHandleGetIncidents_ReturnsAllIncidents(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	seedIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents", nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncidents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []*incidentmodel.Incident
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 1 || got[0].RootService != "atlas-payment-service" {
		t.Fatalf("expected 1 incident rooted at atlas-payment-service, got %+v", got)
	}
}

func TestHandleGetOpenIncidents_ReturnsOnlyOpen(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	seedIncident(manager, "atlas-order-service")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/open", nil)
	rec := httptest.NewRecorder()
	api.HandleGetOpenIncidents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []*incidentmodel.Incident
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 1 || got[0].Status != incidentmodel.StatusOpen {
		t.Fatalf("expected 1 open incident, got %+v", got)
	}
}

func TestHandleGetIncident_Found_ReturnsIncidentDetail(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	id := seedIncident(manager, "atlas-inventory-service")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+id, nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncident(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got incidentmodel.Incident
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if got.IncidentID != id {
		t.Errorf("expected IncidentID %q, got %q", id, got.IncidentID)
	}
}

func TestHandleGetIncident_NotFound_Returns404(t *testing.T) {
	api, _ := newTestIncidentAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/does-not-exist", nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncident(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown incidentId, got %d", rec.Code)
	}
}

func TestHandleGetIncident_EvidenceSubpath_ReturnsAttachedEvidence(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	id := seedIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+id+"/evidence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncident(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got []evidence.Evidence
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got) != 1 || got[0].EvidenceID != "ev-seed-1" {
		t.Fatalf("expected the seeded evidence to be returned, got %+v", got)
	}
}

func TestHandleGetIncident_EvidenceSubpath_UnknownIncident_Returns404(t *testing.T) {
	api, _ := newTestIncidentAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/does-not-exist/evidence", nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncident(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown incidentId, got %d", rec.Code)
	}
}

func TestHandleGetIncident_RCASubpath_ReturnsUnknownWhenNoRCAYetComputed(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	id := seedIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+id+"/rca", nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncident(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got struct {
		RootCause  string `json:"rootCause"`
		Confidence string `json:"confidence"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	// rca.Engine.Analyze() has not run against this incident (that's the
	// background evaluation loop's job in main.go, not this handler's) --
	// HandleGetIncident's rca subpath must report the honest UNKNOWN/NONE
	// default rather than fabricate a root cause.
	if got.RootCause != "UNKNOWN" || got.Confidence != "NONE" {
		t.Fatalf("expected UNKNOWN/NONE before RCA has run, got RootCause=%q Confidence=%q", got.RootCause, got.Confidence)
	}
}

func TestHandleGetIncident_AnalysisSubpath_NotFoundBeforeAnalyzeIsTriggered(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	id := seedIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+id+"/analysis", nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncident(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an incident with no AI analysis triggered yet, got %d", rec.Code)
	}
}

// Module 4: regression guard for the routing defect fixed in main.go (the
// production dispatcher, not this handler, was calling the wrong function
// for POST .../analyze). HandleGetIncident's own GET-only behavior for the
// bare /{incidentId} path must remain exactly as it was.
func TestHandleGetIncident_WrongMethod_StillReturns405(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	id := seedIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+id, nil)
	rec := httptest.NewRecorder()
	api.HandleGetIncident(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /api/v1/incidents/{id} (no /analyze subpath), got %d", rec.Code)
	}
}

// Module 4: HandlePostAnalyze itself was always correct -- these are the
// first tests to ever call it directly. Prior to this, only
// aireasoning.Engine.Analyze was tested (bypassing HTTP entirely), which is
// exactly why the main.go routing defect went undetected.
func TestHandlePostAnalyze_Succeeds(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	id := seedIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+id+"/analyze", nil)
	rec := httptest.NewRecorder()
	api.HandlePostAnalyze(rec, req, id)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got aireasoning.AnalysisResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if got.IncidentID != id {
		t.Errorf("expected IncidentID %q, got %q", id, got.IncidentID)
	}
	if got.Provider != "fake" {
		t.Errorf("expected provider=fake (FakeProvider, no real AI call), got %q", got.Provider)
	}
	if got.Model != "fake-model" {
		t.Errorf("expected model=fake-model, got %q", got.Model)
	}
	if got.ExecutiveSummary == "" {
		t.Error("expected a non-empty executive summary from the real Engine.Analyze/FakeProvider path")
	}
}

func TestHandlePostAnalyze_UnknownIncident_Returns404(t *testing.T) {
	api, _ := newTestIncidentAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/does-not-exist/analyze", nil)
	rec := httptest.NewRecorder()
	api.HandlePostAnalyze(rec, req, "does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown incidentId, got %d", rec.Code)
	}
}

// Module 4: a real incident with its EvidenceIDs cleared via the public
// GetIncident/UpdateIncident round trip (the same technique used in
// internal/serviceintel's tests) -- never a fabricated incident -- to prove
// what the real FakeProvider/Validator pipeline actually does with missing
// evidence. Traced directly from source (not assumed): FakeProvider
// deliberately references a placeholder evidence ID ("E999999", see its own
// comment "[t]o trigger validation failure intentionally in tests if they
// don't provide evidence") when given zero evidence; Validator.Validate
// then rejects that as an unknown/unsourced reference, and Engine.Analyze
// returns ErrValidationFailed, which HandlePostAnalyze reports as 503. This
// is the honest outcome: the system refuses to return an analysis it cannot
// ground in real evidence, rather than fabricating one. A 200 here would
// have meant either evidence was fabricated to satisfy the grounding check,
// or the grounding check silently doesn't apply -- neither is true.
func TestHandlePostAnalyze_MissingEvidence_ReportsHonestly(t *testing.T) {
	api, manager := newTestIncidentAPI(t)
	id := seedIncident(manager, "atlas-payment-service")

	inc := manager.GetIncident(id)
	inc.EvidenceIDs = []string{}
	manager.UpdateIncident(inc)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+id+"/analyze", nil)
	rec := httptest.NewRecorder()
	api.HandlePostAnalyze(rec, req, id)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 -- the real evidence-grounding validator must honestly refuse an ungrounded analysis rather than fabricate one -- got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ai analysis failed validation") {
		t.Errorf("expected the real ErrValidationFailed message, got %q -- this must be the real validator rejecting an ungrounded reference, not an unrelated error", rec.Body.String())
	}
}
