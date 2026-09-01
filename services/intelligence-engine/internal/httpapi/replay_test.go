package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	aiprovider "github.com/atlas/intelligence-engine/internal/aireasoning/provider"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/remediation"
	rmprovider "github.com/atlas/intelligence-engine/internal/remediation/provider"
	"github.com/atlas/intelligence-engine/internal/replay"
)

func newTestReplayAPI(t *testing.T) (*ReplayAPI, *incidentmanager.Manager) {
	t.Helper()
	evStore := evidence.NewStore()
	depGraph := graph.NewDependencyGraph(300)
	manager := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evStore)

	aiCfg := aireasoning.Config{Enabled: true, MaxEvents: 200, MaxStringLength: 1024, TimeoutSeconds: 30, RetentionSeconds: 3600}
	aiProv := aiprovider.NewFakeProvider()
	aiEngine := aireasoning.NewEngine(aiCfg, aiProv)

	rmCfg := remediation.Config{Enabled: true, RetentionSeconds: 3600}
	rmProv := rmprovider.NewFakePlanner()
	rmPlanner := remediation.NewPlanner(rmCfg, rmProv)

	sim := replay.NewSimulator(manager, evStore, depGraph, aiEngine, aiCfg, aiProv, rmPlanner, rmCfg, rmProv)
	return NewReplayAPI(sim), manager
}

func seedReplayableIncident(manager *incidentmanager.Manager, service string) string {
	manager.ProcessSignal(incidentsignal.Signal{
		SignalID:  "sig-replay-http-1",
		Type:      incidentsignal.SignalTypeErrorRate,
		Timestamp: time.Now(),
		Service:   service,
		Operation: "http post /api/orders",
		Value:     0.9,
		Threshold: 0.5,
		Evidence: evidence.Evidence{
			EvidenceID:  "ev-replay-http-1",
			Type:        evidence.EvidenceTypeErrorRate,
			Service:     service,
			Description: "error rate exceeded threshold",
		},
	})
	open := manager.GetOpenIncidents()
	if len(open) != 1 {
		panic("seedReplayableIncident: expected exactly 1 open incident after ProcessSignal")
	}
	id := open[0].IncidentID
	inc := manager.GetIncident(id)
	inc.RCA = &incidentmodel.RootCause{Service: service, Operation: "N/A", Confidence: "MEDIUM", Score: 45}
	inc.Confidence = "MEDIUM"
	inc.Score = 45
	manager.UpdateIncident(inc)
	return id
}

func TestHandleReplayIncident_WrongMethod_Returns405(t *testing.T) {
	api, manager := newTestReplayAPI(t)
	id := seedReplayableIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/"+id+"/replay", nil)
	rec := httptest.NewRecorder()
	api.HandleReplayIncident(rec, req, id)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleReplayIncident_UnknownIncident_Returns404(t *testing.T) {
	api, _ := newTestReplayAPI(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/does-not-exist/replay", nil)
	rec := httptest.NewRecorder()
	api.HandleReplayIncident(rec, req, "does-not-exist")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandleReplayIncident_Succeeds(t *testing.T) {
	api, manager := newTestReplayAPI(t)
	id := seedReplayableIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+id+"/replay", nil)
	rec := httptest.NewRecorder()
	api.HandleReplayIncident(rec, req, id)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got["sourceIncidentId"] != id {
		t.Errorf("sourceIncidentId = %v, want %v", got["sourceIncidentId"], id)
	}
	if got["simulation"] != true {
		t.Errorf("expected simulation=true, got %v", got["simulation"])
	}
	if got["executionPerformed"] != false {
		t.Errorf("expected executionPerformed=false, got %v", got["executionPerformed"])
	}
	if got["approvalPerformed"] != false {
		t.Errorf("expected approvalPerformed=false, got %v", got["approvalPerformed"])
	}
	replayAnalysis, ok := got["replayAnalysis"].(map[string]any)
	if !ok || replayAnalysis["succeeded"] != true {
		t.Fatalf("expected replayAnalysis.succeeded=true, got %+v", got["replayAnalysis"])
	}
	analysisResult, ok := replayAnalysis["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected replayAnalysis.result to be present, got %+v", replayAnalysis)
	}
	if analysisResult["provider"] != "fake" || analysisResult["model"] != "fake-model" {
		t.Errorf("expected provider=fake/model=fake-model, got provider=%v model=%v", analysisResult["provider"], analysisResult["model"])
	}
	replayPlan, ok := got["replayPlan"].(map[string]any)
	if !ok || replayPlan["succeeded"] != true {
		t.Fatalf("expected replayPlan.succeeded=true, got %+v", got["replayPlan"])
	}
	plan, ok := replayPlan["plan"].(map[string]any)
	if !ok {
		t.Fatalf("expected replayPlan.plan to be present, got %+v", replayPlan)
	}
	// Structural proof the plan was never approved: Planner.GeneratePlan
	// itself always sets Status=VALIDATED right before returning (confirmed
	// from source), regardless of provider -- only Planner.ApprovePlan,
	// which replay never calls, would move it to APPROVED.
	if plan["status"] != "VALIDATED" {
		t.Errorf("expected a freshly-generated plan to be VALIDATED (never approved by replay), got status=%v", plan["status"])
	}
	if approval, ok := plan["approval"].(map[string]any); ok {
		if _, approved := approval["approvedAt"]; approved {
			t.Error("replay-generated plan must never carry an approvedAt -- replay never approves anything")
		}
	}
}

func TestHandleReplayIncident_NoUndocumentedTopLevelFields(t *testing.T) {
	api, manager := newTestReplayAPI(t)
	id := seedReplayableIncident(manager, "atlas-payment-service")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+id+"/replay", nil)
	rec := httptest.NewRecorder()
	api.HandleReplayIncident(rec, req, id)

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	allowed := map[string]bool{
		"replayId": true, "sourceIncidentId": true, "replayTimestamp": true,
		"simulation": true, "executionPerformed": true, "approvalPerformed": true,
		"historicalRCA": true, "evidence": true, "dependencies": true,
		"replayAnalysis": true, "historicalAnalysis": true,
		"replayPlan": true, "historicalPlan": true,
	}
	for key := range got {
		if !allowed[key] {
			t.Errorf("unexpected top-level field %q -- no field beyond the documented replay contract should be exposed", key)
		}
	}
}
