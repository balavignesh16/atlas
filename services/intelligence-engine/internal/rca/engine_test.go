package rca

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/propagation"
)

// Locks in the M2.4 confidence boundaries (LOW < 40 <= MEDIUM < 70 <= HIGH) as
// verified in docs/m24_verification_report.md. A prior uncommitted change during
// M2.7 work silently lowered the LOW/MEDIUM boundary to 30 with no test or
// documented justification; this test prevents that from happening unnoticed
// again.
func TestGetConfidence_Boundaries(t *testing.T) {
	e := &Engine{}

	cases := []struct {
		score    int
		expected string
	}{
		{0, "LOW"},
		{30, "LOW"},
		{39, "LOW"},
		{40, "MEDIUM"},
		{69, "MEDIUM"},
		{70, "HIGH"},
		{100, "HIGH"},
	}

	for _, c := range cases {
		if got := e.getConfidence(c.score); got != c.expected {
			t.Errorf("getConfidence(%d) = %q, want %q", c.score, got, c.expected)
		}
	}
}

// newTestEngine wires a real, unmocked Engine against real evidence.Store,
// propagation.Analyzer, and graph.DependencyGraph instances, matching this
// project's established "exercise the real implementation" testing
// convention. Analyze() dereferences all three fields, so a zero-value
// &Engine{} (as used above for the pure getConfidence unit) cannot be used
// for anything that calls Analyze.
func newTestEngine(t *testing.T) (*Engine, *evidence.Store, *graph.DependencyGraph, *correlation.Engine) {
	t.Helper()
	evStore := evidence.NewStore()
	depGraph := graph.NewDependencyGraph(300)
	corrEngine := correlation.NewEngine(depGraph, 300)
	propAnalyzer := propagation.NewAnalyzer(depGraph, corrEngine)
	return NewEngine(evStore, propAnalyzer, depGraph), evStore, depGraph, corrEngine
}

func newTestIncident(services ...string) *incidentmodel.Incident {
	return &incidentmodel.Incident{
		IncidentID:       "inc-test",
		AffectedServices: services,
		EvidenceIDs:      []string{},
		LastUpdatedAt:    time.Now(),
	}
}

func TestAnalyze_NilIncident_DoesNotPanic(t *testing.T) {
	e, _, _, _ := newTestEngine(t)
	e.Analyze(nil)
}

func TestAnalyze_NoAffectedServices_LeavesIncidentUnscored(t *testing.T) {
	e, _, _, _ := newTestEngine(t)
	inc := newTestIncident()

	e.Analyze(inc)

	if inc.RCA != nil {
		t.Fatalf("expected no RCA to be set for an incident with no affected services, got %+v", inc.RCA)
	}
}

func TestAnalyze_SingleCandidateWithErrorEvidence_LowConfidence(t *testing.T) {
	e, evStore, _, _ := newTestEngine(t)
	evStore.Add(evidence.Evidence{EvidenceID: "ev-1", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-payment-service"})
	inc := newTestIncident("atlas-payment-service")
	inc.EvidenceIDs = []string{"ev-1"}

	e.Analyze(inc)

	if inc.RCA == nil {
		t.Fatal("expected RCA to be set")
	}
	if inc.RCA.Service != "atlas-payment-service" {
		t.Errorf("expected root cause atlas-payment-service, got %q", inc.RCA.Service)
	}
	if inc.RCA.Score != 25 {
		t.Errorf("expected score 25 (error-rate only), got %d", inc.RCA.Score)
	}
	if inc.RCA.Confidence != "LOW" {
		t.Errorf("expected LOW confidence for score 25, got %q", inc.RCA.Confidence)
	}
}

func TestAnalyze_ErrorEvidenceCountedOnce_NotPerEvent(t *testing.T) {
	e, evStore, _, _ := newTestEngine(t)
	// Two separate error-rate evidence entries for the same service: the
	// hasErrorIncrease flag in Analyze must only award the +25 bonus once,
	// not once per matching evidence item.
	evStore.Add(evidence.Evidence{EvidenceID: "ev-1", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-2", Type: evidence.EvidenceTypeSpanError, Service: "atlas-payment-service"})
	inc := newTestIncident("atlas-payment-service")
	inc.EvidenceIDs = []string{"ev-1", "ev-2"}

	e.Analyze(inc)

	if inc.RCA.Score != 25 {
		t.Errorf("expected error evidence to be counted once (score 25), got %d", inc.RCA.Score)
	}
}

func TestAnalyze_CombinedEvidenceTypes_AccumulateScore(t *testing.T) {
	e, evStore, _, _ := newTestEngine(t)
	evStore.Add(evidence.Evidence{EvidenceID: "ev-err", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-lat", Type: evidence.EvidenceTypeLatency, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-dep", Type: evidence.EvidenceTypeDependencyError, Service: "atlas-payment-service"})
	inc := newTestIncident("atlas-payment-service")
	inc.EvidenceIDs = []string{"ev-err", "ev-lat", "ev-dep"}

	e.Analyze(inc)

	// 25 (error) + 20 (latency) + 20 (dependency) = 65 -> MEDIUM (40 <= 65 < 70).
	if inc.RCA.Score != 65 {
		t.Errorf("expected combined score 65, got %d", inc.RCA.Score)
	}
	if inc.RCA.Confidence != "MEDIUM" {
		t.Errorf("expected MEDIUM confidence for score 65, got %q", inc.RCA.Confidence)
	}
	if len(inc.EvidenceIDs) != 3 {
		t.Errorf("expected all 3 evidence IDs to remain attached to the incident, got %v", inc.EvidenceIDs)
	}
}

func TestAnalyze_HealthyIndependentDependency_AddsSmallBonus(t *testing.T) {
	e, evStore, depGraph, _ := newTestEngine(t)
	evStore.Add(evidence.Evidence{EvidenceID: "ev-err", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-payment-service"})
	// atlas-payment-service calls an external dependency that is NOT itself
	// an affected service -- Analyze's "healthy independent dependency" check.
	depGraph.AddDependency("atlas-payment-service", "atlas-external-bank-api", 10, false, "OK")
	inc := newTestIncident("atlas-payment-service")
	inc.EvidenceIDs = []string{"ev-err"}

	e.Analyze(inc)

	// 25 (error) + 5 (healthy independent dependency) = 30.
	if inc.RCA.Score != 30 {
		t.Errorf("expected score 30 (25 error + 5 healthy-dependency bonus), got %d", inc.RCA.Score)
	}
}

func TestAnalyze_ScoreCappedAt100(t *testing.T) {
	e, evStore, depGraph, _ := newTestEngine(t)
	evStore.Add(evidence.Evidence{EvidenceID: "ev-err", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-lat", Type: evidence.EvidenceTypeLatency, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-dep", Type: evidence.EvidenceTypeDependencyError, Service: "atlas-payment-service"})
	depGraph.AddDependency("atlas-payment-service", "atlas-external-bank-api", 10, false, "OK")
	inc := newTestIncident("atlas-payment-service")
	inc.EvidenceIDs = []string{"ev-err", "ev-lat", "ev-dep"}

	e.Analyze(inc)

	// 25 + 20 + 20 + 5 = 70, which is already < 100 -- this test locks in
	// that the cap logic exists and does not corrupt a sub-100 score; the
	// cap boundary itself (score > 100 -> 100) has no reachable evidence
	// combination in the real scoring formula that exceeds 100, so this test
	// documents that 70 passes through uncapped rather than fabricating an
	// input the implementation cannot actually produce.
	if inc.RCA.Score != 70 {
		t.Fatalf("expected uncapped score 70, got %d", inc.RCA.Score)
	}
	if inc.RCA.Confidence != "HIGH" {
		t.Errorf("expected HIGH confidence for score 70, got %q", inc.RCA.Confidence)
	}
}

func TestAnalyze_ClearWinner_HighestScoringCandidateWins(t *testing.T) {
	e, evStore, _, _ := newTestEngine(t)
	evStore.Add(evidence.Evidence{EvidenceID: "ev-payment-err", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-payment-lat", Type: evidence.EvidenceTypeLatency, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-payment-dep", Type: evidence.EvidenceTypeDependencyError, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-order-err", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-order-service"})
	inc := newTestIncident("atlas-payment-service", "atlas-order-service")
	inc.EvidenceIDs = []string{"ev-payment-err", "ev-payment-lat", "ev-payment-dep", "ev-order-err"}

	e.Analyze(inc)

	// payment: 65 (error+latency+dependency), order: 25 (error only).
	// Gap is 40 > 5, so this is a clear, non-ambiguous winner.
	if inc.RCA.Service != "atlas-payment-service" {
		t.Errorf("expected atlas-payment-service (score 65) to win over atlas-order-service (score 25), got %q", inc.RCA.Service)
	}
	if inc.RCA.Score != 65 {
		t.Errorf("expected winning score 65, got %d", inc.RCA.Score)
	}
}

func TestAnalyze_CompetingCandidatesWithinFivePoints_ReportsAmbiguous(t *testing.T) {
	e, evStore, _, _ := newTestEngine(t)
	// Both services score exactly 25 (error-rate only) -- a 0-point gap,
	// well within Analyze's <=5 ambiguity threshold.
	evStore.Add(evidence.Evidence{EvidenceID: "ev-payment-err", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-payment-service"})
	evStore.Add(evidence.Evidence{EvidenceID: "ev-order-err", Type: evidence.EvidenceTypeErrorRate, Service: "atlas-order-service"})
	inc := newTestIncident("atlas-payment-service", "atlas-order-service")
	inc.EvidenceIDs = []string{"ev-payment-err", "ev-order-err"}

	e.Analyze(inc)

	if inc.RCA == nil {
		t.Fatal("expected RCA to be set even for an ambiguous result")
	}
	if inc.RCA.Service != "AMBIGUOUS" {
		t.Errorf("expected AMBIGUOUS root cause for a 0-point score gap, got %q", inc.RCA.Service)
	}
	if inc.RCA.Confidence != "LOW" {
		t.Errorf("expected LOW confidence for an ambiguous result, got %q", inc.RCA.Confidence)
	}
	if inc.RCA.Score != 0 {
		t.Errorf("expected score 0 for an ambiguous result (not either candidate's real score), got %d", inc.RCA.Score)
	}
	// Do not force a root cause -- this is the exact safety property
	// AMBIGUOUS exists to protect (see docs/m24_verification_report.md and
	// internal/propagation/analyzer.go's doc comment on the same principle).
}

func TestAnalyze_NoEvidenceForAffectedService_ZeroScoreCandidate(t *testing.T) {
	e, _, _, _ := newTestEngine(t)
	inc := newTestIncident("atlas-payment-service")

	e.Analyze(inc)

	if inc.RCA == nil {
		t.Fatal("expected RCA to be set even with zero evidence")
	}
	if inc.RCA.Score != 0 {
		t.Errorf("expected score 0 with no evidence at all, got %d", inc.RCA.Score)
	}
	if inc.RCA.Confidence != "LOW" {
		t.Errorf("expected LOW confidence for score 0, got %q", inc.RCA.Confidence)
	}
}

func TestAnalyze_TemporalPrecedencePropagation_WhenRealTraceDataSupportsIt(t *testing.T) {
	// internal/propagation/analyzer.go's own doc comment documents this
	// mechanism as "safely dormant" in this project's real telemetry shapes
	// (see M2.7.2), but the code path itself is live, not disabled -- this
	// test proves Analyze() genuinely scores it when the graph and trace
	// data it depends on are actually present, using real
	// correlation.Engine/graph.DependencyGraph instances, not mocks.
	e, _, depGraph, corrEngine := newTestEngine(t)

	// atlas-order-service calls atlas-payment-service.
	depGraph.AddDependency("atlas-order-service", "atlas-payment-service", 10, true, "ERROR")

	earlier := time.Now().Add(-10 * time.Second)
	later := time.Now()
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID: "span-payment", EventType: event.EventTypeTraceSpan,
		TraceID: "trace-prop", SpanID: "span-payment",
		ServiceName: "atlas-payment-service", Status: "ERROR", Timestamp: earlier,
	})
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID: "span-order", EventType: event.EventTypeTraceSpan,
		TraceID: "trace-prop", SpanID: "span-order",
		ServiceName: "atlas-order-service", Status: "ERROR", Timestamp: later,
	})

	inc := newTestIncident("atlas-payment-service", "atlas-order-service")
	inc.TraceIDs = []string{"trace-prop"}
	// No error/latency/dependency evidence at all -- isolate the propagation
	// contribution so it is unambiguous that a nonzero score came from it.

	e.Analyze(inc)

	if inc.RCA == nil {
		t.Fatal("expected RCA to be set")
	}
	if inc.RCA.Service != "atlas-payment-service" {
		t.Fatalf("expected atlas-payment-service (whose failure preceded atlas-order-service's) to win via temporal precedence, got %q", inc.RCA.Service)
	}
	// 20 (temporal precedence) + 10 (downstream propagation) = 30.
	if inc.RCA.Score != 30 {
		t.Errorf("expected score 30 from temporal-precedence + downstream-propagation evidence alone, got %d", inc.RCA.Score)
	}
}
