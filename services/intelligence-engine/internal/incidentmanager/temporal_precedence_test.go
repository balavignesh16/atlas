package incidentmanager

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/propagation"
	"github.com/atlas/intelligence-engine/internal/rca"
	"github.com/google/uuid"
)

// spanSpec describes one span in a synthetic-but-real nested trace fed
// through a real correlation.Engine, so propagation.Analyzer reads genuine
// span data (real StartTime/EndTime/Status derived via the same
// event.IsErrorStatus + correlationmodel.FromEvent path production uses).
type spanSpec struct {
	spanID       string
	parentSpanID string
	service      string
	startOffset  time.Duration
	durationMs   int64
}

func feedRealTrace(corrEngine *correlation.Engine, traceID string, base time.Time, specs []spanSpec) {
	for _, s := range specs {
		corrEngine.ProcessEvent(event.ATLASEvent{
			EventType:    event.EventTypeTraceSpan,
			TraceID:      traceID,
			SpanID:       s.spanID,
			ParentSpanID: s.parentSpanID,
			ServiceName:  s.service,
			Timestamp:    base.Add(s.startOffset),
			DurationMs:   s.durationMs,
			Status:       "UNSET", // matches this project's real server-span telemetry
			Attributes:   map[string]string{"status": "500"},
		})
	}
}

func newFullRCAStack(g *graph.DependencyGraph) (*rca.Engine, *evidence.Store, *correlation.Engine) {
	return newRCAEngine(g)
}

func addDependencyErrorEvidence(evStore *evidence.Store, inc *incidentmodel.Incident, service string) {
	ev := evidence.Evidence{
		EvidenceID:  uuid.New().String(),
		Type:        evidence.EvidenceTypeDependencyError,
		Timestamp:   inc.StartedAt,
		Service:     service,
		Description: service + " dependency failure rate exceeded threshold",
	}
	evStore.Add(ev)
	inc.EvidenceIDs = append(inc.EvidenceIDs, ev.EvidenceID)
}

// TestPropagation_NestedCascade_PrecedenceIntentionallyStaysDormant is a
// regression test locking in a deliberate M2.7.2 decision: temporal
// precedence was investigated (StartTime -> EndTime comparison) and found
// to work correctly in isolation, but was reverted after live testing
// showed it lets a middle-tier caller -- which already carries its own
// DEPENDENCY_FAILURE evidence from rca.Engine's existing, unmodified
// scoring -- combine that with a newly-active precedence bonus to
// confidently outrank the true root cause (a pure sink can never earn the
// dependency-failure bonus). See propagation/analyzer.go's doc comment for
// the full account. This test proves the dormant (StartTime-based, current
// production) behavior: even with fully real, correctly-classified trace
// data for a genuine nested cascade, no candidate receives a precedence
// bonus -- exactly as before M2.7.2 touched anything here.
func TestPropagation_NestedCascade_PrecedenceIntentionallyStaysDormant(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-gateway", "atlas-order-service", 10, true, "5xx")
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, true, "5xx")

	_, _, corrEngine := newFullRCAStack(g)
	propAnalyzer := propagation.NewAnalyzer(g, corrEngine)

	base := time.Now()
	traceID := "trace-3hop"
	feedRealTrace(corrEngine, traceID, base, []spanSpec{
		{spanID: "gw-span", service: "atlas-gateway", startOffset: 0, durationMs: 300},
		{spanID: "order-span", parentSpanID: "gw-span", service: "atlas-order-service", startOffset: 10 * time.Millisecond, durationMs: 250},
		{spanID: "pay-span", parentSpanID: "order-span", service: "atlas-payment-service", startOffset: 20 * time.Millisecond, durationMs: 5},
	})

	inc := newIncident("atlas-payment-service", base)
	inc.TraceIDs = []string{traceID}
	affected := []string{"atlas-gateway", "atlas-order-service", "atlas-payment-service"}

	for _, candidate := range affected {
		evs := propAnalyzer.CheckPropagation(candidate, affected, inc)
		if len(evs) != 0 {
			t.Fatalf("expected temporal precedence to remain dormant for %s (reverted, StartTime-based), got %d pieces of evidence -- if this now fires, the M2.7.2 revert was undone without addressing the rca.Engine interaction it was reverted for", candidate, len(evs))
		}
	}
}

// TestPropagation_FalsePrecedence_OutermostCallerNeverWinsFromRawTiming is
// the explicit false-precedence protection required by M2.7.2: gateway's
// span always starts (and, in this construction, is even given the
// opportunity to appear temporally "early" by any measure) before
// payment's failure is observed, but gateway must never receive the
// root-cause bonus merely for having observed the request first. This
// holds regardless of the StartTime/EndTime revert above -- it verifies
// the IsPath direction gate itself, which neither change touched.
func TestPropagation_FalsePrecedence_OutermostCallerNeverWinsFromRawTiming(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-gateway", "atlas-payment-service", 10, true, "5xx")

	_, _, corrEngine := newFullRCAStack(g)
	propAnalyzer := propagation.NewAnalyzer(g, corrEngine)

	base := time.Now()
	traceID := "trace-false-precedence"
	feedRealTrace(corrEngine, traceID, base, []spanSpec{
		{spanID: "gw-span", service: "atlas-gateway", startOffset: 0, durationMs: 1},
		{spanID: "pay-span", parentSpanID: "gw-span", service: "atlas-payment-service", startOffset: 5 * time.Millisecond, durationMs: 100},
	})

	inc := newIncident("atlas-gateway", base)
	inc.TraceIDs = []string{traceID}

	affected := []string{"atlas-gateway", "atlas-payment-service"}
	gwEvs := propAnalyzer.CheckPropagation("atlas-gateway", affected, inc)
	if len(gwEvs) != 0 {
		t.Fatalf("gateway must NEVER receive temporal precedence evidence regardless of raw timestamps, got %d pieces of evidence", len(gwEvs))
	}
}

// TestRCA_NestedCascadeWithDependencyEvidence_StaysAmbiguousNotConfidentlyWrong
// directly reproduces, as a controlled regression test, the dangerous live
// scenario M2.7.2 found and reverted: gateway and order-service each carry
// their own error-rate AND dependency-failure evidence (exactly what real
// traffic produces, since evaluateDependencies independently flags a
// failing outgoing call for every caller in the chain), while payment (a
// pure sink) only ever has error-rate. With temporal precedence correctly
// left dormant, this must land on AMBIGUOUS -- never a confident,
// specific, wrong answer -- preserving M2.4's "no guessing" principle
// through the real rca.Engine, unmodified.
func TestRCA_NestedCascadeWithDependencyEvidence_StaysAmbiguousNotConfidentlyWrong(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-gateway", "atlas-order-service", 10, true, "5xx")
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, true, "5xx")

	rcaEngine, evStore, corrEngine := newFullRCAStack(g)

	base := time.Now()
	traceID := "trace-3hop-dep"
	feedRealTrace(corrEngine, traceID, base, []spanSpec{
		{spanID: "gw-span", service: "atlas-gateway", startOffset: 0, durationMs: 300},
		{spanID: "order-span", parentSpanID: "gw-span", service: "atlas-order-service", startOffset: 10 * time.Millisecond, durationMs: 250},
		{spanID: "pay-span", parentSpanID: "order-span", service: "atlas-payment-service", startOffset: 20 * time.Millisecond, durationMs: 5},
	})

	inc := newIncident("atlas-payment-service", base)
	inc.AffectedServices = []string{"atlas-gateway", "atlas-order-service", "atlas-payment-service"}
	inc.TraceIDs = []string{traceID}

	// gateway and order each show their own error PLUS a failing outgoing
	// dependency (realistic: evaluateDependencies flags every caller in a
	// failing chain); payment, a pure sink, only ever has its own error.
	addErrorRateEvidence(evStore, inc, "atlas-gateway")
	addDependencyErrorEvidence(evStore, inc, "atlas-gateway")
	addErrorRateEvidence(evStore, inc, "atlas-order-service")
	addDependencyErrorEvidence(evStore, inc, "atlas-order-service")
	addErrorRateEvidence(evStore, inc, "atlas-payment-service")

	rcaEngine.Analyze(inc)

	if inc.RCA == nil {
		t.Fatal("expected an RCA result")
	}
	if inc.RCA.Service != "AMBIGUOUS" {
		t.Fatalf("expected gateway/order's tied evidence (both error+dependency-error, 45 each) to correctly produce AMBIGUOUS, got a confident pick of %q (score=%d) -- this is the exact regression M2.7.2's temporal-precedence attempt caused live and was reverted to prevent", inc.RCA.Service, inc.RCA.Score)
	}
}
