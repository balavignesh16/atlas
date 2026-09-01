package replay

import (
	"context"
	"reflect"
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
)

func newTestSimulator(t *testing.T) (*Simulator, *incidentmanager.Manager, *evidence.Store, *graph.DependencyGraph, *aireasoning.Engine, *remediation.Planner) {
	t.Helper()
	evStore := evidence.NewStore()
	depGraph := graph.NewDependencyGraph(300)
	manager := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evStore)

	aiCfg := aireasoning.Config{
		Enabled:          true,
		MaxEvents:        200,
		MaxStringLength:  1024,
		TimeoutSeconds:   30,
		RetentionSeconds: 3600,
	}
	aiProv := aiprovider.NewFakeProvider()
	aiEngine := aireasoning.NewEngine(aiCfg, aiProv)

	rmCfg := remediation.Config{Enabled: true, RetentionSeconds: 3600}
	rmProv := rmprovider.NewFakePlanner()
	rmPlanner := remediation.NewPlanner(rmCfg, rmProv)

	sim := NewSimulator(manager, evStore, depGraph, aiEngine, aiCfg, aiProv, rmPlanner, rmCfg, rmProv)
	return sim, manager, evStore, depGraph, aiEngine, rmPlanner
}

func seedReplayIncident(manager *incidentmanager.Manager, service string) string {
	manager.ProcessSignal(incidentsignal.Signal{
		SignalID:  "sig-replay-1",
		Type:      incidentsignal.SignalTypeErrorRate,
		Timestamp: time.Now(),
		Service:   service,
		Operation: "http post /api/orders",
		Value:     0.9,
		Threshold: 0.5,
		Evidence: evidence.Evidence{
			EvidenceID:  "ev-replay-1",
			Type:        evidence.EvidenceTypeErrorRate,
			Service:     service,
			Description: "error rate exceeded threshold",
		},
	})
	open := manager.GetOpenIncidents()
	if len(open) != 1 {
		panic("seedReplayIncident: expected exactly 1 open incident after ProcessSignal")
	}
	return open[0].IncidentID
}

// giveIncidentRCA mutates a clone of the incident with a real, non-nil RCA
// verdict and persists it via the public UpdateIncident round trip --
// mirroring how the real background evaluation loop (main.go) does this,
// and the same technique already used in internal/serviceintel's tests.
func giveIncidentRCA(manager *incidentmanager.Manager, id, service, confidence string, score int) {
	inc := manager.GetIncident(id)
	inc.RCA = &incidentmodel.RootCause{Service: service, Operation: "N/A", Confidence: confidence, Score: score}
	inc.Confidence = confidence
	inc.Score = score
	inc.DetectionReason = "error rate exceeded threshold"
	manager.UpdateIncident(inc)
}

func TestSimulate_UnknownIncident_ReturnsNotOk(t *testing.T) {
	sim, _, _, _, _, _ := newTestSimulator(t)
	result, ok, err := sim.Simulate(context.Background(), "does-not-exist", time.Now())
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for an unknown incident, got %+v", result)
	}
}

func TestSimulate_RealIncident_Succeeds(t *testing.T) {
	sim, manager, _, _, _, _ := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	giveIncidentRCA(manager, id, "atlas-payment-service", "MEDIUM", 45)

	result, ok, err := sim.Simulate(context.Background(), id, time.Now())
	if err != nil || !ok {
		t.Fatalf("Simulate: ok=%v err=%v", ok, err)
	}
	if result.SourceIncidentID != id {
		t.Errorf("SourceIncidentID = %q, want %q", result.SourceIncidentID, id)
	}
	if result.ReplayID == "" {
		t.Error("expected a real, non-empty ReplayID")
	}
	if !result.Simulation || result.ExecutionPerformed || result.ApprovalPerformed {
		t.Errorf("expected Simulation=true, ExecutionPerformed=false, ApprovalPerformed=false, got %+v", result)
	}
	if !result.HistoricalRCA.Available || result.HistoricalRCA.Service != "atlas-payment-service" {
		t.Errorf("expected historical RCA available and service=atlas-payment-service, got %+v", result.HistoricalRCA)
	}
	if !result.ReplayAnalysis.Succeeded {
		t.Errorf("expected AI replay to succeed with real evidence present, got %+v", result.ReplayAnalysis)
	}
	if !result.ReplayPlan.Succeeded {
		t.Errorf("expected plan replay to succeed, got %+v", result.ReplayPlan)
	}
}

func TestSimulate_DoesNotMutateSourceIncident(t *testing.T) {
	sim, manager, _, _, _, _ := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	giveIncidentRCA(manager, id, "atlas-payment-service", "MEDIUM", 45)

	before := manager.GetIncident(id)
	if _, _, err := sim.Simulate(context.Background(), id, time.Now()); err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	after := manager.GetIncident(id)

	if !reflect.DeepEqual(before, after) {
		t.Errorf("Simulate mutated the source incident:\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func TestSimulate_DoesNotMutateDependencyGraph(t *testing.T) {
	sim, manager, _, depGraph, _, _ := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	depGraph.AddDependency("gateway", "atlas-payment-service", 10, false, "OK")

	beforeEdges := depGraph.GetEdges()
	if _, _, err := sim.Simulate(context.Background(), id, time.Now()); err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	afterEdges := depGraph.GetEdges()

	if !reflect.DeepEqual(beforeEdges, afterEdges) {
		t.Error("Simulate mutated the live dependency graph's edges")
	}
}

func TestSimulate_AIProviderIdentityIsHonest(t *testing.T) {
	sim, manager, _, _, _, _ := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	giveIncidentRCA(manager, id, "atlas-payment-service", "MEDIUM", 45)

	result, _, err := sim.Simulate(context.Background(), id, time.Now())
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if result.ReplayAnalysis.Result == nil {
		t.Fatal("expected a real AI result")
	}
	if result.ReplayAnalysis.Result.Provider != "fake" || result.ReplayAnalysis.Result.Model != "fake-model" {
		t.Errorf("expected provider=fake/model=fake-model (FakeProvider, no real AI call), got provider=%q model=%q",
			result.ReplayAnalysis.Result.Provider, result.ReplayAnalysis.Result.Model)
	}
}

// Traced against the real FakeProvider/Validator pipeline (see Module 4's
// TestHandlePostAnalyze_MissingEvidence_ReportsHonestly): with zero
// evidence, FakeProvider deliberately references an unresolvable
// placeholder ("E999999"), which the real Validator rejects as ungrounded
// -- so the honest replay outcome is Succeeded=false with a real
// validation-failure reason, not a fabricated success.
func TestSimulate_MissingEvidence_ReportsHonestly(t *testing.T) {
	sim, manager, _, _, _, _ := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	giveIncidentRCA(manager, id, "atlas-payment-service", "MEDIUM", 45)

	inc := manager.GetIncident(id)
	inc.EvidenceIDs = []string{}
	manager.UpdateIncident(inc)

	result, ok, err := sim.Simulate(context.Background(), id, time.Now())
	if err != nil || !ok {
		t.Fatalf("Simulate: ok=%v err=%v", ok, err)
	}
	if result.Evidence.Requested != 0 || result.Evidence.Found != 0 {
		t.Errorf("expected Requested=0 Found=0, got %+v", result.Evidence)
	}
	if result.ReplayAnalysis.Succeeded {
		t.Error("expected AI replay to honestly decline (ungrounded evidence), not fabricate a result")
	}
	if result.ReplayAnalysis.Reason == "" {
		t.Error("expected a real, non-empty reason for the honest decline")
	}
}

func TestSimulate_NoRCAYet_ReportsUnavailable(t *testing.T) {
	sim, manager, _, _, _, _ := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	// Deliberately do NOT call giveIncidentRCA -- RCA has never run.

	result, ok, err := sim.Simulate(context.Background(), id, time.Now())
	if err != nil || !ok {
		t.Fatalf("Simulate: ok=%v err=%v", ok, err)
	}
	if result.HistoricalRCA.Available {
		t.Errorf("expected HistoricalRCA.Available=false when RCA has never run, got %+v", result.HistoricalRCA)
	}
}

// The most important safety proof in this file: replay's own AI/plan
// invocation must never be visible through production's real, shared
// getters -- if it were, a real subsequent call to POST /analyze or
// POST /remediation/plan could observe a "replay" result it never asked
// for.
func TestSimulate_DoesNotContaminateProductionStores(t *testing.T) {
	sim, manager, _, _, aiEngine, rmPlanner := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	giveIncidentRCA(manager, id, "atlas-payment-service", "MEDIUM", 45)

	if _, ok := aiEngine.GetAnalysis(id); ok {
		t.Fatal("test setup invariant violated: production aiEngine already had an analysis before Simulate ran")
	}
	if _, ok := rmPlanner.GetPlanByIncident(id); ok {
		t.Fatal("test setup invariant violated: production rmPlanner already had a plan before Simulate ran")
	}

	if _, _, err := sim.Simulate(context.Background(), id, time.Now()); err != nil {
		t.Fatalf("Simulate: %v", err)
	}

	if _, ok := aiEngine.GetAnalysis(id); ok {
		t.Error("Simulate wrote into production's real, shared AI analysis store -- it must only ever use its own throwaway instance")
	}
	if _, ok := rmPlanner.GetPlanByIncident(id); ok {
		t.Error("Simulate wrote into production's real, shared remediation plan store -- it must only ever use its own throwaway instance")
	}
}

// Once production HAS a real historical analysis/plan (from a genuine
// prior call, simulated here the same way HandlePostAnalyze/HandlePostPlan
// would produce one), replay must surface it for comparison, read-only.
func TestSimulate_SurfacesRealHistoricalAnalysisAndPlanForComparison(t *testing.T) {
	sim, manager, evStore, _, aiEngine, rmPlanner := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	giveIncidentRCA(manager, id, "atlas-payment-service", "MEDIUM", 45)

	inc := manager.GetIncident(id)
	evs := evStore.GetAll(inc.EvidenceIDs)
	evidences := make([]*evidence.Evidence, len(evs))
	for i := range evs {
		evidences[i] = &evs[i]
	}
	historicalAnalysis, err := aiEngine.Analyze(inc, nil, evidences, nil, nil, false)
	if err != nil {
		t.Fatalf("failed to seed a real historical analysis: %v", err)
	}
	historicalPlan, err := rmPlanner.GeneratePlan(context.Background(), inc, historicalAnalysis, evidences, false)
	if err != nil {
		t.Fatalf("failed to seed a real historical plan: %v", err)
	}

	result, ok, err := sim.Simulate(context.Background(), id, time.Now())
	if err != nil || !ok {
		t.Fatalf("Simulate: ok=%v err=%v", ok, err)
	}
	if result.HistoricalAnalysis == nil || result.HistoricalAnalysis.AnalysisID != historicalAnalysis.AnalysisID {
		t.Errorf("expected HistoricalAnalysis to surface the real, already-cached analysis (id=%s), got %+v", historicalAnalysis.AnalysisID, result.HistoricalAnalysis)
	}
	if result.HistoricalPlan == nil || result.HistoricalPlan.PlanID != historicalPlan.PlanID {
		t.Errorf("expected HistoricalPlan to surface the real, already-cached plan (id=%s), got %+v", historicalPlan.PlanID, result.HistoricalPlan)
	}
	// And the replay's OWN analysis/plan must still be a genuinely distinct
	// run, not merely echoing the historical one back.
	if result.ReplayAnalysis.Result != nil && result.ReplayAnalysis.Result.AnalysisID == historicalAnalysis.AnalysisID {
		t.Error("replay's own analysis must be a fresh, distinct run, not the historical one relabeled")
	}
}

// Determinism is claimed only for the substantive analytical content, never
// for identity/timestamp fields that are legitimately fresh on every call
// (AnalysisID/GeneratedAt/PlanID/CreatedAt/ReplayID) -- exactly as true of
// the real, unmodified /analyze and /remediation/plan endpoints today.
func TestSimulate_DeterministicAnalyticalContentGivenIdenticalInputs(t *testing.T) {
	sim, manager, _, _, _, _ := newTestSimulator(t)
	id := seedReplayIncident(manager, "atlas-payment-service")
	giveIncidentRCA(manager, id, "atlas-payment-service", "MEDIUM", 45)

	first, _, err := sim.Simulate(context.Background(), id, time.Now())
	if err != nil {
		t.Fatalf("Simulate (first): %v", err)
	}
	second, _, err := sim.Simulate(context.Background(), id, time.Now())
	if err != nil {
		t.Fatalf("Simulate (second): %v", err)
	}

	if first.ReplayAnalysis.Result.LikelyRootCause != second.ReplayAnalysis.Result.LikelyRootCause {
		t.Errorf("LikelyRootCause differed across identical replays: %q vs %q",
			first.ReplayAnalysis.Result.LikelyRootCause, second.ReplayAnalysis.Result.LikelyRootCause)
	}
	if first.ReplayAnalysis.Result.RootCauseConfidence != second.ReplayAnalysis.Result.RootCauseConfidence {
		t.Errorf("RootCauseConfidence differed across identical replays: %q vs %q",
			first.ReplayAnalysis.Result.RootCauseConfidence, second.ReplayAnalysis.Result.RootCauseConfidence)
	}
	if first.ReplayAnalysis.Result.Provider != second.ReplayAnalysis.Result.Provider {
		t.Error("Provider differed across identical replays")
	}
	if first.ReplayPlan.Plan.Actions[0].TargetService != second.ReplayPlan.Plan.Actions[0].TargetService {
		t.Error("plan target service differed across identical replays")
	}
	// Identity fields are legitimately expected to differ -- asserting they
	// DO differ documents this as an intentional characteristic, not an
	// oversight.
	if first.ReplayID == second.ReplayID {
		t.Error("expected distinct ReplayIDs across two separate Simulate calls")
	}
}
