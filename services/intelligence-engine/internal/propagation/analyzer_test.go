package propagation

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

// newTestAnalyzer wires a real, unmocked Analyzer against real
// graph.DependencyGraph/correlation.Engine instances, matching this
// project's established "exercise the real implementation" testing
// convention.
func newTestAnalyzer(t *testing.T) (*Analyzer, *graph.DependencyGraph, *correlation.Engine) {
	t.Helper()
	depGraph := graph.NewDependencyGraph(300)
	corrEngine := correlation.NewEngine(depGraph, 300)
	return NewAnalyzer(depGraph, corrEngine), depGraph, corrEngine
}

// seedErrorSpan feeds a real ERROR span for service, in traceID, starting at
// ts, through the real correlation.Engine.
func seedErrorSpan(corrEngine *correlation.Engine, traceID, spanID, service string, ts time.Time) {
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID:     "ev-" + spanID,
		EventType:   event.EventTypeTraceSpan,
		TraceID:     traceID,
		SpanID:      spanID,
		ServiceName: service,
		Status:      "ERROR",
		Timestamp:   ts,
	})
}

func newTestIncident(traceIDs ...string) *incidentmodel.Incident {
	return &incidentmodel.Incident{IncidentID: "inc-test", TraceIDs: traceIDs}
}

// ============================================================================
// IsPath
// ============================================================================

func TestIsPath_DirectEdge_ReturnsTrue(t *testing.T) {
	a, depGraph, _ := newTestAnalyzer(t)
	depGraph.AddDependency("gateway", "order", 10, false, "OK")

	if !a.IsPath("gateway", "order") {
		t.Fatal("expected a direct edge to be a path")
	}
}

func TestIsPath_MultiHop_ReturnsTrue(t *testing.T) {
	a, depGraph, _ := newTestAnalyzer(t)
	depGraph.AddDependency("gateway", "order", 10, false, "OK")
	depGraph.AddDependency("order", "payment", 10, false, "OK")

	if !a.IsPath("gateway", "payment") {
		t.Fatal("expected a multi-hop path (gateway->order->payment) to be found via BFS")
	}
}

func TestIsPath_NoEdges_ReturnsFalse(t *testing.T) {
	a, _, _ := newTestAnalyzer(t)

	if a.IsPath("gateway", "payment") {
		t.Fatal("expected no path when the graph has no edges at all")
	}
}

func TestIsPath_DisconnectedGraph_ReturnsFalse(t *testing.T) {
	a, depGraph, _ := newTestAnalyzer(t)
	// Two structurally disconnected components.
	depGraph.AddDependency("gateway", "order", 10, false, "OK")
	depGraph.AddDependency("inventory", "warehouse", 10, false, "OK")

	if a.IsPath("gateway", "warehouse") {
		t.Fatal("expected no path between two structurally disconnected components")
	}
}

func TestIsPath_ReverseDirection_ReturnsFalse(t *testing.T) {
	a, depGraph, _ := newTestAnalyzer(t)
	depGraph.AddDependency("gateway", "order", 10, false, "OK")

	// order does not call gateway -- edges are directional.
	if a.IsPath("order", "gateway") {
		t.Fatal("expected no path in the reverse direction of a directional edge")
	}
}

func TestIsPath_Cycle_TerminatesAndFindsPath(t *testing.T) {
	a, depGraph, _ := newTestAnalyzer(t)
	// A -> B -> C -> A: a real cycle. BFS's visited set must prevent an
	// infinite loop, and still correctly find A -> C via B.
	depGraph.AddDependency("svc-a", "svc-b", 10, false, "OK")
	depGraph.AddDependency("svc-b", "svc-c", 10, false, "OK")
	depGraph.AddDependency("svc-c", "svc-a", 10, false, "OK")

	if !a.IsPath("svc-a", "svc-c") {
		t.Fatal("expected A->C to be found via B despite the graph containing a cycle")
	}
}

// Documents actual behavior, not asserted correctness: IsPath's loop checks
// curr==target before ever consulting edges, so source==target is always
// true regardless of whether any edge exists at all. This characterizes the
// real implementation rather than inventing new semantics.
func TestIsPath_SourceEqualsTarget_ReturnsTrue(t *testing.T) {
	a, _, _ := newTestAnalyzer(t)

	if !a.IsPath("svc-a", "svc-a") {
		t.Fatal("expected IsPath(x, x) to be true (documents actual BFS-loop behavior: curr==target is checked before any edge lookup)")
	}
}

// ============================================================================
// CheckTemporalPrecedence
// ============================================================================

func TestCheckTemporalPrecedence_CandidateFailsFirst_ReturnsTrue(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)
	seedErrorSpan(corrEngine, "trace-1", "span-cand", "payment", earlier)
	seedErrorSpan(corrEngine, "trace-1", "span-target", "order", later)
	inc := newTestIncident("trace-1")

	prec, candTime, targetTime := a.CheckTemporalPrecedence("payment", "order", inc)

	if !prec {
		t.Fatal("expected precedence: candidate (payment) failed strictly before target (order)")
	}
	if !candTime.Equal(earlier) || !targetTime.Equal(later) {
		t.Errorf("expected candTime=%v targetTime=%v, got candTime=%v targetTime=%v", earlier, later, candTime, targetTime)
	}
}

func TestCheckTemporalPrecedence_TargetFailsFirst_ReturnsFalse(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)
	// Reversed: target fails first, candidate fails later -- violates precedence.
	seedErrorSpan(corrEngine, "trace-1", "span-target", "order", earlier)
	seedErrorSpan(corrEngine, "trace-1", "span-cand", "payment", later)
	inc := newTestIncident("trace-1")

	prec, _, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if prec {
		t.Fatal("expected no precedence: candidate failed AFTER target")
	}
}

// Boundary: exactly equal timestamps. time.Before is strict, so equal
// timestamps must not count as precedence.
func TestCheckTemporalPrecedence_EqualTimestamps_ReturnsFalse(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	same := time.Now()
	seedErrorSpan(corrEngine, "trace-1", "span-cand", "payment", same)
	seedErrorSpan(corrEngine, "trace-1", "span-target", "order", same)
	inc := newTestIncident("trace-1")

	prec, _, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if prec {
		t.Fatal("expected no precedence for exactly equal timestamps (Before() is strict, not <=)")
	}
}

func TestCheckTemporalPrecedence_CandidateNeverFails_ReturnsFalse(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	seedErrorSpan(corrEngine, "trace-1", "span-target", "order", time.Now())
	inc := newTestIncident("trace-1")

	prec, _, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if prec {
		t.Fatal("expected no precedence when the candidate has no ERROR span at all")
	}
}

func TestCheckTemporalPrecedence_TargetNeverFails_ReturnsFalse(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	seedErrorSpan(corrEngine, "trace-1", "span-cand", "payment", time.Now())
	inc := newTestIncident("trace-1")

	prec, _, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if prec {
		t.Fatal("expected no precedence when the target has no ERROR span at all")
	}
}

func TestCheckTemporalPrecedence_NonErrorSpansIgnored(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID: "ev-1", EventType: event.EventTypeTraceSpan, TraceID: "trace-1", SpanID: "span-1",
		ServiceName: "payment", Status: "OK", Timestamp: time.Now(),
	})
	seedErrorSpan(corrEngine, "trace-1", "span-2", "order", time.Now().Add(time.Second))
	inc := newTestIncident("trace-1")

	prec, _, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if prec {
		t.Fatal("expected no precedence: candidate's only span is healthy (OK), not an ERROR")
	}
}

// Multiple traces: the earliest ERROR timestamp across ALL of the
// incident's traces must be used, not just the first trace encountered.
func TestCheckTemporalPrecedence_EarliestAcrossMultipleTraces(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	base := time.Now()
	// trace-1: candidate fails late.
	seedErrorSpan(corrEngine, "trace-1", "span-cand-late", "payment", base.Add(10*time.Second))
	// trace-2: candidate fails earlier -- this is the one that should count.
	seedErrorSpan(corrEngine, "trace-2", "span-cand-early", "payment", base)
	seedErrorSpan(corrEngine, "trace-2", "span-target", "order", base.Add(5*time.Second))
	inc := newTestIncident("trace-1", "trace-2")

	prec, candTime, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if !prec {
		t.Fatal("expected precedence using the earliest candidate failure across both traces")
	}
	if !candTime.Equal(base) {
		t.Errorf("expected the earliest candidate timestamp (%v) to be used, got %v", base, candTime)
	}
}

func TestCheckTemporalPrecedence_UnresolvableTraceID_SkippedSafely(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	earlier := time.Now()
	later := earlier.Add(time.Second)
	seedErrorSpan(corrEngine, "trace-real", "span-cand", "payment", earlier)
	seedErrorSpan(corrEngine, "trace-real", "span-target", "order", later)
	inc := newTestIncident("trace-never-ingested", "trace-real")

	prec, _, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if !prec {
		t.Fatal("expected the unresolvable trace ID to be skipped safely, still finding precedence in the real trace")
	}
}

func TestCheckTemporalPrecedence_EmptyTraceIDs_ReturnsFalse(t *testing.T) {
	a, _, _ := newTestAnalyzer(t)
	inc := newTestIncident()

	prec, _, _ := a.CheckTemporalPrecedence("payment", "order", inc)

	if prec {
		t.Fatal("expected no precedence for an incident with no TraceIDs")
	}
}

// ============================================================================
// CheckPropagation
// ============================================================================

func TestCheckPropagation_DirectPathWithPrecedence_ProducesEvidence(t *testing.T) {
	a, depGraph, corrEngine := newTestAnalyzer(t)
	depGraph.AddDependency("order", "payment", 10, false, "OK") // order calls payment
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)
	seedErrorSpan(corrEngine, "trace-1", "span-payment", "payment", earlier)
	seedErrorSpan(corrEngine, "trace-1", "span-order", "order", later)
	inc := newTestIncident("trace-1")

	evs := a.CheckPropagation("payment", []string{"payment", "order"}, inc)

	if len(evs) != 1 {
		t.Fatalf("expected exactly 1 propagation evidence, got %d: %+v", len(evs), evs)
	}
	if evs[0].EvidenceID != "EV-TEMP-payment-order" {
		t.Errorf("expected EvidenceID EV-TEMP-payment-order, got %q", evs[0].EvidenceID)
	}
	if evs[0].Type != evidence.EvidenceTypeTemporalSequence {
		t.Errorf("expected EvidenceTypeTemporalSequence, got %q", evs[0].Type)
	}
	if evs[0].Service != "payment" {
		t.Errorf("expected evidence Service=payment (the candidate), got %q", evs[0].Service)
	}
}

func TestCheckPropagation_NoPath_ProducesNoEvidence(t *testing.T) {
	a, _, corrEngine := newTestAnalyzer(t)
	// No graph edge between order and payment at all.
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)
	seedErrorSpan(corrEngine, "trace-1", "span-payment", "payment", earlier)
	seedErrorSpan(corrEngine, "trace-1", "span-order", "order", later)
	inc := newTestIncident("trace-1")

	evs := a.CheckPropagation("payment", []string{"payment", "order"}, inc)

	if len(evs) != 0 {
		t.Fatalf("expected no evidence when no graph path connects the two services, got %+v", evs)
	}
}

func TestCheckPropagation_PathButNoPrecedence_ProducesNoEvidence(t *testing.T) {
	a, depGraph, corrEngine := newTestAnalyzer(t)
	depGraph.AddDependency("order", "payment", 10, false, "OK")
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)
	// Reversed: order (the caller) fails first, payment (the candidate) fails later.
	seedErrorSpan(corrEngine, "trace-1", "span-order", "order", earlier)
	seedErrorSpan(corrEngine, "trace-1", "span-payment", "payment", later)
	inc := newTestIncident("trace-1")

	evs := a.CheckPropagation("payment", []string{"payment", "order"}, inc)

	if len(evs) != 0 {
		t.Fatalf("expected no evidence when a path exists but temporal precedence does not hold, got %+v", evs)
	}
}

func TestCheckPropagation_MultiHopPath_ProducesEvidence(t *testing.T) {
	a, depGraph, corrEngine := newTestAnalyzer(t)
	depGraph.AddDependency("gateway", "order", 10, false, "OK")
	depGraph.AddDependency("order", "payment", 10, false, "OK")
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)
	seedErrorSpan(corrEngine, "trace-1", "span-payment", "payment", earlier)
	seedErrorSpan(corrEngine, "trace-1", "span-gateway", "gateway", later)
	inc := newTestIncident("trace-1")

	evs := a.CheckPropagation("payment", []string{"payment", "gateway"}, inc)

	if len(evs) != 1 {
		t.Fatalf("expected 1 evidence via the multi-hop path gateway->order->payment, got %d: %+v", len(evs), evs)
	}
}

func TestCheckPropagation_CandidateExcludedFromItsOwnComparison(t *testing.T) {
	a, depGraph, corrEngine := newTestAnalyzer(t)
	depGraph.AddDependency("payment", "payment", 10, false, "OK") // degenerate self-edge, if it existed
	seedErrorSpan(corrEngine, "trace-1", "span-payment", "payment", time.Now())
	inc := newTestIncident("trace-1")

	evs := a.CheckPropagation("payment", []string{"payment"}, inc)

	if len(evs) != 0 {
		t.Fatalf("expected candidate to never be compared against itself, got %+v", evs)
	}
}

func TestCheckPropagation_EmptyAffectedList_ProducesNoEvidence(t *testing.T) {
	a, _, _ := newTestAnalyzer(t)
	inc := newTestIncident("trace-1")

	evs := a.CheckPropagation("payment", []string{}, inc)

	if len(evs) != 0 {
		t.Fatalf("expected no evidence for an empty affected list, got %+v", evs)
	}
}

func TestCheckPropagation_MultipleQualifyingOthers_ProducesEvidenceForEach(t *testing.T) {
	a, depGraph, corrEngine := newTestAnalyzer(t)
	depGraph.AddDependency("order", "payment", 10, false, "OK")
	depGraph.AddDependency("gateway", "payment", 10, false, "OK")
	earlier := time.Now()
	later := earlier.Add(5 * time.Second)
	seedErrorSpan(corrEngine, "trace-1", "span-payment", "payment", earlier)
	seedErrorSpan(corrEngine, "trace-1", "span-order", "order", later)
	seedErrorSpan(corrEngine, "trace-1", "span-gateway", "gateway", later)
	inc := newTestIncident("trace-1")

	evs := a.CheckPropagation("payment", []string{"payment", "order", "gateway"}, inc)

	if len(evs) != 2 {
		t.Fatalf("expected 2 propagation evidences (one per qualifying caller), got %d: %+v", len(evs), evs)
	}
}
