package serviceintel

import (
	"reflect"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/registry"
)

func newTestAssembler(t *testing.T) (*Assembler, *registry.Store, *graph.DependencyGraph, *incidentmanager.Manager) {
	t.Helper()
	store, err := registry.NewStore(":memory:")
	if err != nil {
		t.Fatalf("registry.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	depGraph := graph.NewDependencyGraph(300)
	incManager := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evidence.NewStore())
	return NewAssembler(store, depGraph, incManager), store, depGraph, incManager
}

func newTestSignal(service, operation string, at time.Time) incidentsignal.Signal {
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

func TestBuild_RegistryOnly(t *testing.T) {
	a, store, _, _ := newTestAssembler(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("checkout-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	result, ok, err := a.Build("checkout-service", at.Add(time.Minute))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for a registry-known service")
	}
	if !result.Registry.Known {
		t.Error("expected Registry.Known = true")
	}
	if result.Registry.Status != "ACTIVE" {
		t.Errorf("Registry.Status = %q, want ACTIVE", result.Registry.Status)
	}
	if len(result.Dependencies.Incoming) != 0 || len(result.Dependencies.Outgoing) != 0 {
		t.Errorf("expected no dependencies, got %+v", result.Dependencies)
	}
	if len(result.RelevantIncidents) != 0 {
		t.Errorf("expected no incidents, got %+v", result.RelevantIncidents)
	}
}

func TestBuild_GraphOnly(t *testing.T) {
	a, _, depGraph, _ := newTestAssembler(t)
	// Real dependency edges for a name the registry has NEVER recorded --
	// AddDependency alone never touches the registry (see
	// internal/ingestion for the only real code path that calls both).
	depGraph.AddDependency("gateway", "graph-only-service", 15, false, "OK")

	result, ok, err := a.Build("graph-only-service", time.Now())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for a graph-known (but registry-unknown) service")
	}
	if result.Registry.Known {
		t.Error("expected Registry.Known = false -- this name was never given to the registry")
	}
	if result.Registry.Status != "" || result.Registry.Provenance != "" {
		t.Errorf("expected zero-valued registry fields when unknown, got %+v", result.Registry)
	}
	if len(result.Dependencies.Incoming) != 1 || result.Dependencies.Incoming[0].Service != "gateway" {
		t.Fatalf("expected 1 incoming dependency from gateway, got %+v", result.Dependencies.Incoming)
	}
}

func TestBuild_IncidentOnly(t *testing.T) {
	a, _, _, incManager := newTestAssembler(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	incManager.ProcessSignal(newTestSignal("incident-only-service", "op", at))

	result, ok, err := a.Build("incident-only-service", at.Add(time.Minute))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for a service known only through incident history")
	}
	if result.Registry.Known {
		t.Error("expected Registry.Known = false")
	}
	if len(result.Dependencies.Incoming) != 0 || len(result.Dependencies.Outgoing) != 0 {
		t.Errorf("expected no dependencies, got %+v", result.Dependencies)
	}
	if len(result.RelevantIncidents) != 1 {
		t.Fatalf("expected 1 relevant incident, got %d", len(result.RelevantIncidents))
	}
}

func TestBuild_CombinedEvidence(t *testing.T) {
	a, store, depGraph, incManager := newTestAssembler(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := store.Observe("checkout-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	depGraph.AddDependency("gateway", "checkout-service", 20, false, "OK")
	depGraph.AddDependency("checkout-service", "payment-service", 30, true, "ERROR")
	incManager.ProcessSignal(newTestSignal("checkout-service", "checkout", at))

	result, ok, err := a.Build("checkout-service", at.Add(time.Minute))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if !result.Registry.Known {
		t.Error("expected Registry.Known = true")
	}
	if len(result.Dependencies.Incoming) != 1 || result.Dependencies.Incoming[0].Service != "gateway" {
		t.Errorf("expected 1 incoming dependency from gateway, got %+v", result.Dependencies.Incoming)
	}
	if len(result.Dependencies.Outgoing) != 1 || result.Dependencies.Outgoing[0].Service != "payment-service" {
		t.Errorf("expected 1 outgoing dependency to payment-service, got %+v", result.Dependencies.Outgoing)
	}
	if len(result.RelevantIncidents) != 1 {
		t.Fatalf("expected 1 relevant incident, got %d", len(result.RelevantIncidents))
	}
}

func TestBuild_TotallyUnknown_ReturnsNotOk(t *testing.T) {
	a, _, _, _ := newTestAssembler(t)
	result, ok, err := a.Build("never-heard-of-it", time.Now())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for a name unknown to all three sources, got %+v", result)
	}
}

func TestBuild_DependencyFieldMapping(t *testing.T) {
	a, _, depGraph, _ := newTestAssembler(t)
	depGraph.AddDependency("caller-service", "target-service", 42, false, "OK")
	depGraph.AddDependency("caller-service", "target-service", 8, true, "ERROR")

	result, ok, err := a.Build("target-service", time.Now())
	if err != nil || !ok {
		t.Fatalf("Build: ok=%v err=%v", ok, err)
	}
	if len(result.Dependencies.Incoming) != 1 {
		t.Fatalf("expected 1 aggregated incoming edge, got %d", len(result.Dependencies.Incoming))
	}
	dep := result.Dependencies.Incoming[0]
	if dep.Service != "caller-service" {
		t.Errorf("Service = %q, want caller-service", dep.Service)
	}
	if dep.CallCount != 2 {
		t.Errorf("CallCount = %d, want 2 (aggregated across both calls)", dep.CallCount)
	}
	if dep.ErrorCount != 1 {
		t.Errorf("ErrorCount = %d, want 1", dep.ErrorCount)
	}
	if dep.AverageDurationMs != 25 { // (42+8)/2
		t.Errorf("AverageDurationMs = %d, want 25", dep.AverageDurationMs)
	}
	if dep.FirstObserved.IsZero() || dep.LastObserved.IsZero() {
		t.Errorf("expected non-zero FirstObserved/LastObserved, got %+v", dep)
	}
}

func TestBuild_IncidentAssociation_MatchesRootServiceOrAffectedServices(t *testing.T) {
	a, _, _, incManager := newTestAssembler(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// RootService match.
	incManager.ProcessSignal(newTestSignal("root-match-service", "op", at))

	// AffectedServices-only match: create an incident rooted at a
	// different service, then add our target to AffectedServices via the
	// public GetIncident/UpdateIncident round trip (the only way to
	// construct this precisely without a second real signal source).
	incManager.ProcessSignal(newTestSignal("some-other-root", "op", at))
	for _, inc := range incManager.GetAllIncidents() {
		if inc.RootService == "some-other-root" {
			inc.AffectedServices = append(inc.AffectedServices, "affected-match-service")
			incManager.UpdateIncident(inc)
		}
	}

	// Unrelated incident that must NOT match either target.
	incManager.ProcessSignal(newTestSignal("unrelated-service", "op", at))

	rootResult, ok, err := a.Build("root-match-service", at.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("Build(root-match-service): ok=%v err=%v", ok, err)
	}
	if len(rootResult.RelevantIncidents) != 1 {
		t.Fatalf("expected exactly 1 incident matched by RootService, got %d", len(rootResult.RelevantIncidents))
	}

	affectedResult, ok, err := a.Build("affected-match-service", at.Add(time.Minute))
	if err != nil || !ok {
		t.Fatalf("Build(affected-match-service): ok=%v err=%v", ok, err)
	}
	if len(affectedResult.RelevantIncidents) != 1 {
		t.Fatalf("expected exactly 1 incident matched by AffectedServices, got %d", len(affectedResult.RelevantIncidents))
	}

	unrelatedResult, ok, err := a.Build("no-such-service-anywhere", at.Add(time.Minute))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for a name that matches no incident, registry, or graph entry, got %+v", unrelatedResult)
	}
}

func TestBuild_IncidentLimit_BoundedMostRecentFirstDeterministic(t *testing.T) {
	a, _, _, incManager := newTestAssembler(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// 15 distinct incidents, all rooted at the same service, at
	// increasing timestamps -- more than the 10-item limit.
	for i := 0; i < 15; i++ {
		at := base.Add(time.Duration(i) * time.Hour)
		incManager.ProcessSignal(newTestSignal("busy-service", "op", at))
		// ProcessSignal dedupes by fingerprint into the same OPEN incident
		// unless we force distinct ones -- resolve each immediately after
		// creating it so the next signal starts a new incident.
		for _, inc := range incManager.GetAllIncidents() {
			if inc.RootService == "busy-service" && inc.Status == "OPEN" {
				inc.Status = "RESOLVED"
				incManager.UpdateIncident(inc)
			}
		}
	}

	result, ok, err := a.Build("busy-service", base.Add(100*time.Hour))
	if err != nil || !ok {
		t.Fatalf("Build: ok=%v err=%v", ok, err)
	}
	if len(result.RelevantIncidents) != relevantIncidentsLimit {
		t.Fatalf("expected exactly %d incidents (bounded), got %d", relevantIncidentsLimit, len(result.RelevantIncidents))
	}
	for i := 1; i < len(result.RelevantIncidents); i++ {
		if result.RelevantIncidents[i].StartedAt.After(result.RelevantIncidents[i-1].StartedAt) {
			t.Fatalf("expected most-recent-first ordering, violated at index %d", i)
		}
	}
	// The most recent incident (i=14, at base+14h) must be first.
	wantLatest := base.Add(14 * time.Hour)
	if !result.RelevantIncidents[0].StartedAt.Equal(wantLatest) {
		t.Errorf("expected the most recent incident (%v) first, got %v", wantLatest, result.RelevantIncidents[0].StartedAt)
	}
}

func TestBuild_Deterministic_IdenticalInputsProduceEqualResults(t *testing.T) {
	a, store, depGraph, incManager := newTestAssembler(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := store.Observe("multi-dep-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	for _, caller := range []string{"caller-a", "caller-b", "caller-c"} {
		depGraph.AddDependency(caller, "multi-dep-service", 10, false, "OK")
	}
	incManager.ProcessSignal(newTestSignal("multi-dep-service", "op", at))

	generatedAt := at.Add(time.Hour)
	first, ok1, err1 := a.Build("multi-dep-service", generatedAt)
	second, ok2, err2 := a.Build("multi-dep-service", generatedAt)

	if err1 != nil || err2 != nil || !ok1 || !ok2 {
		t.Fatalf("Build: ok1=%v err1=%v ok2=%v err2=%v", ok1, err1, ok2, err2)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Build is not deterministic for identical inputs:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}

func TestBuild_EmptySlicesAreNeverNil(t *testing.T) {
	a, store, _, _ := newTestAssembler(t)
	if err := store.Observe("lonely-service", time.Now()); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	result, ok, err := a.Build("lonely-service", time.Now())
	if err != nil || !ok {
		t.Fatalf("Build: ok=%v err=%v", ok, err)
	}
	if result.Dependencies.Incoming == nil {
		t.Error("expected Dependencies.Incoming to be an empty slice, not nil")
	}
	if result.Dependencies.Outgoing == nil {
		t.Error("expected Dependencies.Outgoing to be an empty slice, not nil")
	}
	if result.RelevantIncidents == nil {
		t.Error("expected RelevantIncidents to be an empty slice, not nil")
	}
}

func TestBuild_DoesNotMutateSources(t *testing.T) {
	a, store, depGraph, incManager := newTestAssembler(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := store.Observe("stable-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	depGraph.AddDependency("gateway", "stable-service", 10, false, "OK")
	incManager.ProcessSignal(newTestSignal("stable-service", "op", at))

	before, _, _ := store.Get("stable-service")
	beforeIncoming, beforeOutgoing := depGraph.GetServiceDependencies("stable-service")
	beforeIncidents := incManager.GetAllIncidents()

	if _, _, err := a.Build("stable-service", at.Add(time.Hour)); err != nil {
		t.Fatalf("Build: %v", err)
	}

	after, _, _ := store.Get("stable-service")
	if !reflect.DeepEqual(before, after) {
		t.Errorf("Build mutated the registry record:\nbefore: %+v\nafter:  %+v", before, after)
	}
	afterIncoming, afterOutgoing := depGraph.GetServiceDependencies("stable-service")
	if !reflect.DeepEqual(beforeIncoming, afterIncoming) || !reflect.DeepEqual(beforeOutgoing, afterOutgoing) {
		t.Error("Build mutated the graph's dependency edges")
	}
	afterIncidents := incManager.GetAllIncidents()
	if len(beforeIncidents) != len(afterIncidents) {
		t.Errorf("Build changed the incident count: before=%d after=%d", len(beforeIncidents), len(afterIncidents))
	}
}
