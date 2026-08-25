package incidentmanager

import (
	"strings"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

// These tests run the real, UNMODIFIED rca.Engine.Analyze (via newRCAEngine,
// already defined in correlator_test.go) after real Correlator.Correlate and
// real CausalAnalyzer.ApplyCausalAttribution, proving the full M2.7.1 ->
// M2.7.3 -> rca.Engine pipeline behaves correctly end to end without any
// change to rca.Engine's code.

// TestCausal_A_LinearCascade_PaymentWinsCleanly is the milestone's primary
// correctness example: Gateway -> Order -> Payment, Payment is the true
// root. Before this milestone, Order/Gateway tied at 45 (AMBIGUOUS) and
// Payment never entered contention at 25. After causal redirection, both
// callers' DEPENDENCY_ERROR evidence moves to Payment.
func TestCausal_A_LinearCascade_PaymentWinsCleanly(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-gateway", "atlas-order-service", 20, 15)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)

	rcaEngine, evStore, _ := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	gateway := newIncident("atlas-gateway", base.Add(2*time.Second))
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	payment := newIncident("atlas-payment-service", base) // true root

	addErrorRateEvidence(evStore, gateway, "atlas-gateway")
	addErrorRateEvidence(evStore, order, "atlas-order-service")
	addErrorRateEvidence(evStore, payment, "atlas-payment-service")
	addDependencyErrorEvidence(evStore, gateway, "atlas-gateway")
	addDependencyErrorEvidence(evStore, order, "atlas-order-service")

	incidents := []*incidentmodel.Incident{gateway, order, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(3*time.Second))
	causal.ApplyCausalAttribution(incidents, g, evStore)

	var primary *incidentmodel.Incident
	for _, inc := range incidents {
		if inc.PrimaryIncidentID == inc.IncidentID {
			primary = inc
		}
	}
	if primary == nil {
		t.Fatal("expected exactly one primary")
	}

	rcaEngine.Analyze(primary)

	if primary.RCA == nil {
		t.Fatal("expected an RCA result")
	}
	if primary.RCA.Service != "atlas-payment-service" {
		t.Fatalf("expected Payment to win cleanly, got %q (score=%d, reason=%s)", primary.RCA.Service, primary.RCA.Score, primary.DetectionReason)
	}
	if primary.RCA.Confidence == "LOW" {
		t.Fatalf("expected at least MEDIUM confidence for a clean causal win, got LOW (score=%d)", primary.RCA.Score)
	}
}

// TestCausal_C_DependencyVictim_ExtraLocalEvidenceStillCannotOutrankRoot
// gives Order BOTH its own error-rate AND latency evidence (a stronger
// "many local errors" case than scenario A), and Payment only its own
// error-rate plus the redirected dependency credit. Order must never
// STRICTLY outrank Payment.
func TestCausal_C_DependencyVictim_ExtraLocalEvidenceStillCannotOutrankRoot(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)

	rcaEngine, evStore, _ := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	payment := newIncident("atlas-payment-service", base)

	addErrorRateEvidence(evStore, order, "atlas-order-service")
	addLatencyEvidence(evStore, order, "atlas-order-service") // extra local evidence
	addDependencyErrorEvidence(evStore, order, "atlas-order-service")
	addErrorRateEvidence(evStore, payment, "atlas-payment-service")

	incidents := []*incidentmodel.Incident{order, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(2*time.Second))
	causal.ApplyCausalAttribution(incidents, g, evStore)

	var primary *incidentmodel.Incident
	for _, inc := range incidents {
		if inc.PrimaryIncidentID == inc.IncidentID {
			primary = inc
		}
	}
	rcaEngine.Analyze(primary)

	if primary.RCA.Service == "atlas-order-service" {
		t.Fatalf("order-service must never outrank the true root cause solely from local evidence volume, got order selected (score=%d, reason=%s)", primary.RCA.Score, primary.DetectionReason)
	}
}

// TestCausal_D_TrulyIndependentFailures_NoCrossAttribution: Payment and
// Inventory have no graph relationship and no shared caller at all. They
// must not be correlated (M2.7.1, unchanged) and causal attribution must
// not invent a relationship between them either -- each resolves
// independently and correctly to itself.
func TestCausal_D_TrulyIndependentFailures_NoCrossAttribution(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	// no edges at all between these two services

	rcaEngine, evStore, _ := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	payment := newIncident("atlas-payment-service", base)
	inventory := newIncident("atlas-inventory-service", base)

	addErrorRateEvidence(evStore, payment, "atlas-payment-service")
	addErrorRateEvidence(evStore, inventory, "atlas-inventory-service")

	incidents := []*incidentmodel.Incident{payment, inventory}
	NewCorrelator(20).Correlate(incidents, g, base)
	causal.ApplyCausalAttribution(incidents, g, evStore)

	if payment.CorrelationGroupID == inventory.CorrelationGroupID {
		t.Fatal("truly unrelated incidents must never be correlated (M2.7.1 behavior)")
	}

	rcaEngine.Analyze(payment)
	rcaEngine.Analyze(inventory)

	if payment.RCA.Service != "atlas-payment-service" {
		t.Fatalf("expected payment's own incident to correctly name itself, got %q", payment.RCA.Service)
	}
	if inventory.RCA.Service != "atlas-inventory-service" {
		t.Fatalf("expected inventory's own incident to correctly name itself, got %q", inventory.RCA.Service)
	}
}

// TestCausal_E_SharedCaller_AmbiguousBetweenTrueSinksOrderExcluded is the
// explicitly required regression test: Order depends on both Payment and
// Inventory, both fail with equivalent evidence. Expected: Payment and
// Inventory are the two legitimate, equally-credited candidates and the
// result is AMBIGUOUS between them -- Order must not appear as a candidate
// in the ambiguous pair, and must not win outright either.
func TestCausal_E_SharedCaller_AmbiguousBetweenTrueSinksOrderExcluded(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)
	seedEdge(g, "atlas-order-service", "atlas-inventory-service", 20, 15)

	rcaEngine, evStore, _ := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	payment := newIncident("atlas-payment-service", base)
	inventory := newIncident("atlas-inventory-service", base)

	addErrorRateEvidence(evStore, order, "atlas-order-service")
	addDependencyErrorEvidence(evStore, order, "atlas-order-service") // order->payment edge
	addDependencyErrorEvidence(evStore, order, "atlas-order-service") // order->inventory edge (2nd, distinct DEPENDENCY_ERROR item)
	addErrorRateEvidence(evStore, payment, "atlas-payment-service")
	addErrorRateEvidence(evStore, inventory, "atlas-inventory-service")

	incidents := []*incidentmodel.Incident{order, payment, inventory}
	NewCorrelator(20).Correlate(incidents, g, base.Add(2*time.Second))
	causal.ApplyCausalAttribution(incidents, g, evStore)

	var primary *incidentmodel.Incident
	for _, inc := range incidents {
		if inc.PrimaryIncidentID == inc.IncidentID {
			primary = inc
		}
	}
	rcaEngine.Analyze(primary)

	if primary.RCA.Service != "AMBIGUOUS" {
		t.Fatalf("expected Payment and Inventory (equally credited true sinks) to produce AMBIGUOUS, got %q (score=%d)", primary.RCA.Service, primary.RCA.Score)
	}
	if strings.Contains(primary.DetectionReason, "atlas-order-service") {
		t.Fatalf("order-service (the shared caller, not a true sink) must never appear as an ambiguous candidate, got reason: %s", primary.DetectionReason)
	}
}

// TestCausal_F_GenuineLocalRoot_StillWinsAlone confirms zero regression for
// an isolated, single-service failure (no cascade at all) -- matches
// M2.7.2's already-verified live behavior.
func TestCausal_F_GenuineLocalRoot_StillWinsAlone(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	rcaEngine, evStore, _ := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	payment := newIncident("atlas-payment-service", base)
	addErrorRateEvidence(evStore, payment, "atlas-payment-service")

	incidents := []*incidentmodel.Incident{payment}
	NewCorrelator(20).Correlate(incidents, g, base)
	causal.ApplyCausalAttribution(incidents, g, evStore)

	rcaEngine.Analyze(payment)

	if payment.RCA.Service != "atlas-payment-service" {
		t.Fatalf("expected an isolated genuine local failure to still correctly name itself, got %q", payment.RCA.Service)
	}
}

// TestCausal_G_IncompleteGraph_NoInventedRelationship: order's dependency
// failure edge exists, but payment (its target) is not part of the current
// correlated group (e.g. not yet detected/correlated). Must not invent a
// causal link to a service that isn't observably part of this incident.
func TestCausal_G_IncompleteGraph_NoInventedRelationship(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)

	rcaEngine, evStore, _ := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	order := newIncident("atlas-order-service", base)
	addErrorRateEvidence(evStore, order, "atlas-order-service")
	addDependencyErrorEvidence(evStore, order, "atlas-order-service")

	// Only order is tracked as an incident; payment is deliberately absent.
	incidents := []*incidentmodel.Incident{order}
	NewCorrelator(20).Correlate(incidents, g, base)
	causal.ApplyCausalAttribution(incidents, g, evStore)

	rcaEngine.Analyze(order)

	if order.RCA.Service != "atlas-order-service" {
		t.Fatalf("expected order's own dependency evidence to remain attributed to it when payment isn't part of the group, got %q", order.RCA.Service)
	}
}

// TestCausal_H_Cycle_SafeFallbackThroughRealRCA confirms the cycle fallback
// (§causal.go) doesn't crash or fabricate a result when driven through the
// real rca.Engine.
func TestCausal_H_Cycle_SafeFallbackThroughRealRCA(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "service-a", "service-b", 20, 15)
	seedEdge(g, "service-b", "service-a", 20, 15)

	rcaEngine, evStore, _ := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	a := newIncident("service-a", base)
	b := newIncident("service-b", base)
	addErrorRateEvidence(evStore, a, "service-a")
	addErrorRateEvidence(evStore, b, "service-b")
	addDependencyErrorEvidence(evStore, a, "service-a")
	addDependencyErrorEvidence(evStore, b, "service-b")

	incidents := []*incidentmodel.Incident{a, b}
	NewCorrelator(20).Correlate(incidents, g, base)
	causal.ApplyCausalAttribution(incidents, g, evStore) // must not panic

	var primary *incidentmodel.Incident
	for _, inc := range incidents {
		if inc.PrimaryIncidentID == inc.IncidentID {
			primary = inc
		}
	}
	rcaEngine.Analyze(primary) // must not panic; result unconstrained but must exist
	if primary.RCA == nil {
		t.Fatal("expected a safe (even if ambiguous) RCA result for a cyclic graph, not a crash or nil result")
	}
}

// TestCausal_J_TemporalEvidencePresent_PropagationStaysDormant confirms
// causal attribution and (still-dormant, per M2.7.2) temporal precedence
// don't interact -- the propagation bonus contributes nothing regardless of
// whether real trace data is present, proving the deferral documented in
// propagation/analyzer.go still holds after this milestone's changes.
func TestCausal_J_TemporalEvidencePresent_PropagationStaysDormant(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-gateway", "atlas-order-service", 20, 15)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)

	rcaEngine, evStore, corrEngine := newRCAEngine(g)
	causal := newTestCausalAnalyzer()

	base := time.Now()
	traceID := "trace-1"
	feedRealTrace(corrEngine, traceID, base, []spanSpec{
		{spanID: "gw-span", service: "atlas-gateway", startOffset: 0, durationMs: 300},
		{spanID: "order-span", parentSpanID: "gw-span", service: "atlas-order-service", startOffset: 10 * time.Millisecond, durationMs: 250},
		{spanID: "pay-span", parentSpanID: "order-span", service: "atlas-payment-service", startOffset: 20 * time.Millisecond, durationMs: 5},
	})

	gateway := newIncident("atlas-gateway", base.Add(2*time.Second))
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	payment := newIncident("atlas-payment-service", base)
	gateway.TraceIDs = []string{traceID}
	order.TraceIDs = []string{traceID}
	payment.TraceIDs = []string{traceID}

	addErrorRateEvidence(evStore, gateway, "atlas-gateway")
	addErrorRateEvidence(evStore, order, "atlas-order-service")
	addErrorRateEvidence(evStore, payment, "atlas-payment-service")
	addDependencyErrorEvidence(evStore, gateway, "atlas-gateway")
	addDependencyErrorEvidence(evStore, order, "atlas-order-service")

	incidents := []*incidentmodel.Incident{gateway, order, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(3*time.Second))
	causal.ApplyCausalAttribution(incidents, g, evStore)

	var primary *incidentmodel.Incident
	for _, inc := range incidents {
		if inc.PrimaryIncidentID == inc.IncidentID {
			primary = inc
		}
	}
	rcaEngine.Analyze(primary)

	// Same result as TestCausal_A (which has no trace data at all) --
	// proves temporal evidence being present changes nothing.
	if primary.RCA.Service != "atlas-payment-service" {
		t.Fatalf("expected the same causal-attribution-driven result regardless of trace data presence, got %q", primary.RCA.Service)
	}
	if primary.RCA.Score >= 70 {
		t.Fatalf("score reaching HIGH (%d) would suggest the dormant propagation bonus fired -- it must not, per M2.7.2's deferral", primary.RCA.Score)
	}
}
