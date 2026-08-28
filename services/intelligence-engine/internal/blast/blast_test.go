package blast

import (
	"testing"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

// newTestAnalyzer wires a real, unmocked Analyzer against a real
// correlation.Engine, matching this project's established "exercise the
// real implementation" testing convention.
func newTestAnalyzer(t *testing.T) (*Analyzer, *correlation.Engine) {
	t.Helper()
	depGraph := graph.NewDependencyGraph(300)
	corrEngine := correlation.NewEngine(depGraph, 300)
	return NewAnalyzer(corrEngine), corrEngine
}

// seedTrace feeds a single real span through the real correlation.Engine so
// Calculate operates on a genuinely reconstructed trace, not a fabricated
// DTO. status is the span's OTel-style status ("OK", "ERROR", "UNSET").
func seedTrace(corrEngine *correlation.Engine, traceID, spanID, service, status string) {
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID:       "ev-" + spanID,
		EventType:     event.EventTypeTraceSpan,
		TraceID:       traceID,
		SpanID:        spanID,
		ServiceName:   service,
		OperationName: "op-" + service,
		Status:        status,
	})
}

// seedChildTrace feeds a span that is a child of parentSpanID within the
// same trace, for edge-building coverage.
func seedChildTrace(corrEngine *correlation.Engine, traceID, spanID, parentSpanID, service, status string) {
	corrEngine.ProcessEvent(event.ATLASEvent{
		EventID:       "ev-" + spanID,
		EventType:     event.EventTypeTraceSpan,
		TraceID:       traceID,
		SpanID:        spanID,
		ParentSpanID:  parentSpanID,
		ServiceName:   service,
		OperationName: "op-" + service,
		Status:        status,
	})
}

func TestCalculate_NilIncident_DoesNotPanic(t *testing.T) {
	a, _ := newTestAnalyzer(t)
	a.Calculate(nil)
}

// No TraceIDs at all: Calculate must return immediately, leaving every
// field at its zero value rather than fabricating counts from nothing.
func TestCalculate_NoTraceIDs_LeavesIncidentAtZeroValues(t *testing.T) {
	a, _ := newTestAnalyzer(t)
	inc := &incidentmodel.Incident{IncidentID: "inc-1", TraceIDs: []string{}}

	a.Calculate(inc)

	if inc.TraceCount != 0 {
		t.Errorf("expected TraceCount 0 for an incident with no TraceIDs, got %d", inc.TraceCount)
	}
	if inc.FailureCount != 0 {
		t.Errorf("expected FailureCount 0 for an incident with no TraceIDs, got %d", inc.FailureCount)
	}
	if inc.AffectedServices != nil {
		t.Errorf("expected AffectedServices to remain untouched (nil), got %v", inc.AffectedServices)
	}
}

// Exactly one trace, and it is failing: the minimal single-trace case,
// isolated from any multi-trace interaction.
func TestCalculate_SingleFailingTrace_ReportsOneAndOne(t *testing.T) {
	a, corrEngine := newTestAnalyzer(t)
	seedTrace(corrEngine, "trace-1", "span-1", "atlas-payment-service", "ERROR")

	inc := &incidentmodel.Incident{IncidentID: "inc-single", TraceIDs: []string{"trace-1"}}
	a.Calculate(inc)

	if inc.TraceCount != 1 {
		t.Errorf("expected TraceCount 1 for a single trace, got %d", inc.TraceCount)
	}
	if inc.FailureCount != 1 {
		t.Errorf("expected FailureCount 1 for a single failing trace, got %d", inc.FailureCount)
	}
}

// M2.14 regression: this is the exact confirmed defect. Two genuinely
// distinct, real, all-ERROR traces must produce TraceCount=2 and
// FailureCount=2. Against the pre-fix implementation (which computed
// failureCount locally but never assigned it to inc.FailureCount, and never
// set inc.TraceCount at all), this test fails with TraceCount=0,
// FailureCount=0 despite two real failing traces -- exactly the
// "traceCount: 0, failureCount: 0" defect observed live via the incident
// API.
func TestCalculate_MultipleErrorTraces_ReportsCorrectCounts(t *testing.T) {
	a, corrEngine := newTestAnalyzer(t)
	seedTrace(corrEngine, "trace-1", "span-1", "atlas-payment-service", "ERROR")
	seedTrace(corrEngine, "trace-2", "span-2", "atlas-payment-service", "ERROR")

	inc := &incidentmodel.Incident{IncidentID: "inc-2", TraceIDs: []string{"trace-1", "trace-2"}}
	a.Calculate(inc)

	if inc.TraceCount != 2 {
		t.Errorf("expected TraceCount 2, got %d", inc.TraceCount)
	}
	if inc.FailureCount != 2 {
		t.Errorf("expected FailureCount 2 (both traces are ERROR), got %d", inc.FailureCount)
	}
}

// All traces healthy (OK): TraceCount must reflect the real trace count,
// but FailureCount must remain 0 -- a healthy trace is not a failure.
func TestCalculate_AllSuccessTraces_ZeroFailureCount(t *testing.T) {
	a, corrEngine := newTestAnalyzer(t)
	seedTrace(corrEngine, "trace-1", "span-1", "atlas-order-service", "OK")
	seedTrace(corrEngine, "trace-2", "span-2", "atlas-order-service", "OK")

	inc := &incidentmodel.Incident{IncidentID: "inc-3", TraceIDs: []string{"trace-1", "trace-2"}}
	a.Calculate(inc)

	if inc.TraceCount != 2 {
		t.Errorf("expected TraceCount 2, got %d", inc.TraceCount)
	}
	if inc.FailureCount != 0 {
		t.Errorf("expected FailureCount 0 for all-healthy traces, got %d", inc.FailureCount)
	}
}

// Mixed success and error traces: FailureCount must count only the ERROR
// trace, not the healthy one.
func TestCalculate_MixedSuccessAndErrorTraces_CorrectCounts(t *testing.T) {
	a, corrEngine := newTestAnalyzer(t)
	seedTrace(corrEngine, "trace-ok", "span-ok", "atlas-order-service", "OK")
	seedTrace(corrEngine, "trace-err", "span-err", "atlas-payment-service", "ERROR")

	inc := &incidentmodel.Incident{IncidentID: "inc-4", TraceIDs: []string{"trace-ok", "trace-err"}}
	a.Calculate(inc)

	if inc.TraceCount != 2 {
		t.Errorf("expected TraceCount 2, got %d", inc.TraceCount)
	}
	if inc.FailureCount != 1 {
		t.Errorf("expected FailureCount 1 (only trace-err is ERROR), got %d", inc.FailureCount)
	}
}

// A TraceID present on the incident but never resolvable via
// corrEngine.GetTrace (e.g. aged out of correlation retention, or never
// actually ingested) must not be counted in TraceCount -- TraceCount
// reflects traces the calculation actually processed, not len(TraceIDs).
func TestCalculate_UnresolvableTraceID_NotCountedInTraceCount(t *testing.T) {
	a, corrEngine := newTestAnalyzer(t)
	seedTrace(corrEngine, "trace-real", "span-1", "atlas-payment-service", "ERROR")

	inc := &incidentmodel.Incident{IncidentID: "inc-5", TraceIDs: []string{"trace-real", "trace-never-ingested"}}
	a.Calculate(inc)

	if inc.TraceCount != 1 {
		t.Errorf("expected TraceCount 1 (only the resolvable trace counts, not len(TraceIDs)=2), got %d", inc.TraceCount)
	}
	if inc.FailureCount != 1 {
		t.Errorf("expected FailureCount 1, got %d", inc.FailureCount)
	}
}

// No TraceID at all resolves (all stale/unknown): both counts must be 0,
// not a crash or a fabricated non-zero value.
func TestCalculate_NoTraceIDsResolve_BothCountsZero(t *testing.T) {
	a, _ := newTestAnalyzer(t)
	inc := &incidentmodel.Incident{IncidentID: "inc-6", TraceIDs: []string{"unknown-1", "unknown-2"}}

	a.Calculate(inc)

	if inc.TraceCount != 0 {
		t.Errorf("expected TraceCount 0 when no TraceIDs resolve, got %d", inc.TraceCount)
	}
	if inc.FailureCount != 0 {
		t.Errorf("expected FailureCount 0 when no TraceIDs resolve, got %d", inc.FailureCount)
	}
}

// Existing, already-correct behavior (must remain unchanged by this fix):
// ERROR spans populate AffectedServices/AffectedOperations, and a
// parent->child ERROR span pair populates AffectedEdges.
func TestCalculate_ErrorSpans_PopulateAffectedServicesOperationsEdges(t *testing.T) {
	a, corrEngine := newTestAnalyzer(t)
	seedTrace(corrEngine, "trace-1", "span-parent", "atlas-order-service", "ERROR")
	seedChildTrace(corrEngine, "trace-1", "span-child", "span-parent", "atlas-payment-service", "ERROR")

	inc := &incidentmodel.Incident{IncidentID: "inc-7", TraceIDs: []string{"trace-1"}}
	a.Calculate(inc)

	if len(inc.AffectedServices) != 2 {
		t.Errorf("expected 2 affected services, got %v", inc.AffectedServices)
	}
	if len(inc.AffectedOperations) != 2 {
		t.Errorf("expected 2 affected operations, got %v", inc.AffectedOperations)
	}
	if len(inc.AffectedEdges) != 1 || inc.AffectedEdges[0] != "atlas-order-service->atlas-payment-service" {
		t.Errorf("expected 1 edge atlas-order-service->atlas-payment-service, got %v", inc.AffectedEdges)
	}
	// This fix's own scope: counts must also be correct alongside the
	// pre-existing edge-building behavior.
	if inc.TraceCount != 1 || inc.FailureCount != 1 {
		t.Errorf("expected TraceCount=1 FailureCount=1, got TraceCount=%d FailureCount=%d", inc.TraceCount, inc.FailureCount)
	}
}

// A healthy (OK) span must never populate AffectedServices/Operations/Edges
// -- only ERROR/5xx spans do. Existing behavior, unchanged by this fix.
func TestCalculate_HealthySpans_DoNotPopulateAffected(t *testing.T) {
	a, corrEngine := newTestAnalyzer(t)
	seedTrace(corrEngine, "trace-1", "span-1", "atlas-order-service", "OK")

	inc := &incidentmodel.Incident{IncidentID: "inc-8", TraceIDs: []string{"trace-1"}}
	a.Calculate(inc)

	if len(inc.AffectedServices) != 0 {
		t.Errorf("expected no affected services for an all-healthy trace, got %v", inc.AffectedServices)
	}
	if len(inc.AffectedEdges) != 0 {
		t.Errorf("expected no affected edges for an all-healthy trace, got %v", inc.AffectedEdges)
	}
}
