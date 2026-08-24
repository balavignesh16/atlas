package incidentmanager

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/propagation"
	"github.com/atlas/intelligence-engine/internal/rca"
	"github.com/google/uuid"
)

func newIncident(service string, startedAt time.Time) *incidentmodel.Incident {
	id := uuid.New().String()
	return &incidentmodel.Incident{
		IncidentID:       id,
		Status:           incidentmodel.StatusOpen,
		StartedAt:        startedAt,
		LastUpdatedAt:    startedAt,
		RootService:      service,
		AffectedServices: []string{service},
		EvidenceIDs:      []string{},
	}
}

func TestCorrelate_LinearChainPicksCalleeAsPrimary(t *testing.T) {
	// Gateway -> Order -> Payment. Payment is the only sink: it doesn't call
	// anything else in the group, so it must become primary regardless of
	// start-time ordering.
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-gateway", "atlas-order-service", 10, false, "OK")
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, false, "OK")

	base := time.Now()
	gateway := newIncident("atlas-gateway", base)
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	payment := newIncident("atlas-payment-service", base.Add(2*time.Second)) // started LAST, must still win

	incidents := []*incidentmodel.Incident{gateway, order, payment}

	c := NewCorrelator(20)
	c.Correlate(incidents, g, base.Add(3*time.Second))

	if payment.PrimaryIncidentID != payment.IncidentID {
		t.Fatalf("expected payment (the callee/sink) to be primary, got primaryIncidentId=%s on payment", payment.PrimaryIncidentID)
	}
	if gateway.PrimaryIncidentID != payment.IncidentID {
		t.Fatalf("expected gateway to point at payment as primary, got %s", gateway.PrimaryIncidentID)
	}
	if order.PrimaryIncidentID != payment.IncidentID {
		t.Fatalf("expected order to point at payment as primary, got %s", order.PrimaryIncidentID)
	}
	if gateway.CorrelationGroupID == "" || gateway.CorrelationGroupID != order.CorrelationGroupID || order.CorrelationGroupID != payment.CorrelationGroupID {
		t.Fatalf("expected all three incidents to share one correlationGroupId")
	}

	wantAffected := map[string]bool{"atlas-gateway": true, "atlas-order-service": true, "atlas-payment-service": true}
	if len(payment.AffectedServices) != len(wantAffected) {
		t.Fatalf("expected payment's AffectedServices to be the union of the cascade, got %v", payment.AffectedServices)
	}
	for _, s := range payment.AffectedServices {
		if !wantAffected[s] {
			t.Fatalf("unexpected service %q in payment's merged AffectedServices", s)
		}
	}

	// non-primary incidents are left exactly as M2.4 produced them
	if len(gateway.AffectedServices) != 1 || gateway.AffectedServices[0] != "atlas-gateway" {
		t.Fatalf("expected gateway's own AffectedServices to be untouched, got %v", gateway.AffectedServices)
	}
}

func TestCorrelate_CallerNeverBecomesPrimaryOverItsCallee(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, false, "OK")

	base := time.Now()
	// Order started failing first -- a naive "earliest start wins" rule
	// would incorrectly pick Order. The causal rule must still pick Payment.
	order := newIncident("atlas-order-service", base)
	payment := newIncident("atlas-payment-service", base.Add(5*time.Second))

	incidents := []*incidentmodel.Incident{order, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(6*time.Second))

	if payment.PrimaryIncidentID != payment.IncidentID {
		t.Fatalf("caller (order) must never outrank its callee (payment) as primary; got primaryIncidentId=%s", order.PrimaryIncidentID)
	}
}

func TestCorrelate_UnconnectedIncidentsNeverMerge(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	// no edges added -- these two services have never been observed calling each other

	base := time.Now()
	a := newIncident("atlas-payment-service", base)
	b := newIncident("atlas-notification-service", base)

	incidents := []*incidentmodel.Incident{a, b}
	NewCorrelator(20).Correlate(incidents, g, base)

	if a.CorrelationGroupID == b.CorrelationGroupID {
		t.Fatalf("unconnected incidents must never share a correlationGroupId")
	}
	if a.PrimaryIncidentID != a.IncidentID || b.PrimaryIncidentID != b.IncidentID {
		t.Fatalf("unconnected incidents must each remain their own primary")
	}
	if len(a.RelatedIncidentIDs) != 0 || len(b.RelatedIncidentIDs) != 0 {
		t.Fatalf("unconnected incidents must have no related incidents")
	}
}

func TestCorrelate_OutsideTimeWindowNeverMerge(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, false, "OK")

	base := time.Now()
	order := newIncident("atlas-order-service", base)
	payment := newIncident("atlas-payment-service", base.Add(100*time.Second)) // far outside a 20s window

	incidents := []*incidentmodel.Incident{order, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(100*time.Second))

	if order.CorrelationGroupID == payment.CorrelationGroupID {
		t.Fatalf("incidents outside the correlation window must not be merged")
	}
}

func TestCorrelate_ParallelIndependentFailuresTieBreakByEarliestStart(t *testing.T) {
	// Order calls both Payment and Inventory; Payment and Inventory have no
	// edge between them. Both are valid sink candidates -- the tie-break
	// must pick whichever started first, and it must never pick Order.
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, false, "OK")
	g.AddDependency("atlas-order-service", "atlas-inventory-service", 10, false, "OK")

	base := time.Now()
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	inventory := newIncident("atlas-inventory-service", base) // starts first
	payment := newIncident("atlas-payment-service", base.Add(2*time.Second))

	incidents := []*incidentmodel.Incident{order, inventory, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(3*time.Second))

	if inventory.PrimaryIncidentID != inventory.IncidentID {
		t.Fatalf("expected the earliest-started sink candidate (inventory) to be primary, got primaryIncidentId=%s", inventory.PrimaryIncidentID)
	}
	if order.PrimaryIncidentID == order.IncidentID {
		t.Fatalf("order calls both failing services in the group and must never be primary")
	}
}

func TestCorrelate_CycleFallsBackWithoutPanicking(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("service-a", "service-b", 10, false, "OK")
	g.AddDependency("service-b", "service-a", 10, false, "OK") // observed call cycle

	base := time.Now()
	a := newIncident("service-a", base.Add(1*time.Second))
	b := newIncident("service-b", base) // starts first

	incidents := []*incidentmodel.Incident{a, b}
	NewCorrelator(20).Correlate(incidents, g, base.Add(2*time.Second))

	if a.CorrelationGroupID == "" || a.CorrelationGroupID != b.CorrelationGroupID {
		t.Fatalf("expected the cycle to still be grouped via the documented fallback")
	}
	if b.PrimaryIncidentID != b.IncidentID {
		t.Fatalf("expected the fallback to pick the earliest-started incident (b) as primary, got %s", b.PrimaryIncidentID)
	}
}

func TestCorrelate_FailsOpenOnNilGraph(t *testing.T) {
	base := time.Now()
	a := newIncident("atlas-payment-service", base)

	incidents := []*incidentmodel.Incident{a}
	NewCorrelator(20).Correlate(incidents, nil, base)

	if a.CorrelationGroupID != "" {
		t.Fatalf("expected fail-open behavior (no correlation applied) when depGraph is nil")
	}
}

func TestCorrelate_SingleIncidentGetsSelfReferentialMetadata(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	base := time.Now()
	a := newIncident("atlas-payment-service", base)

	incidents := []*incidentmodel.Incident{a}
	NewCorrelator(20).Correlate(incidents, g, base)

	if a.CorrelationGroupID == "" {
		t.Fatalf("expected a standalone incident to still receive a correlationGroupId")
	}
	if a.PrimaryIncidentID != a.IncidentID {
		t.Fatalf("expected a standalone incident's primaryIncidentId to be itself, got %s", a.PrimaryIncidentID)
	}
	if len(a.RelatedIncidentIDs) != 0 {
		t.Fatalf("expected a standalone incident to have no related incidents, got %v", a.RelatedIncidentIDs)
	}
}

// --- Integration tests against the real, UNMODIFIED rca.Engine ---
// These prove correlation happens before RCA and that RCA's existing,
// frozen scoring/ambiguity logic needs zero changes to correctly evaluate a
// cascade once the primary incident carries the merged evidence.

func newRCAEngine(depGraph *graph.DependencyGraph) (*rca.Engine, *evidence.Store, *correlation.Engine) {
	corrEngine := correlation.NewEngine(depGraph, 300)
	propAnalyzer := propagation.NewAnalyzer(depGraph, corrEngine)
	evStore := evidence.NewStore()
	return rca.NewEngine(evStore, propAnalyzer, depGraph), evStore, corrEngine
}

func addErrorRateEvidence(evStore *evidence.Store, inc *incidentmodel.Incident, service string) {
	ev := evidence.Evidence{
		EvidenceID:  uuid.New().String(),
		Type:        evidence.EvidenceTypeErrorRate,
		Timestamp:   inc.StartedAt,
		Service:     service,
		Description: service + " error rate exceeded threshold",
	}
	evStore.Add(ev)
	inc.EvidenceIDs = append(inc.EvidenceIDs, ev.EvidenceID)
}

func addLatencyEvidence(evStore *evidence.Store, inc *incidentmodel.Incident, service string) {
	ev := evidence.Evidence{
		EvidenceID:  uuid.New().String(),
		Type:        evidence.EvidenceTypeLatency,
		Timestamp:   inc.StartedAt,
		Service:     service,
		Description: service + " latency exceeded threshold",
	}
	evStore.Add(ev)
	inc.EvidenceIDs = append(inc.EvidenceIDs, ev.EvidenceID)
}

func TestCorrelate_RCASeesFullCascadeAfterCorrelation(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-gateway", "atlas-order-service", 10, true, "5xx")
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, true, "5xx")

	rcaEngine, evStore, _ := newRCAEngine(g)

	base := time.Now()
	gateway := newIncident("atlas-gateway", base.Add(2*time.Second))
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	payment := newIncident("atlas-payment-service", base) // true root, started first too

	// Payment gets BOTH elevated error rate and elevated latency (it's the
	// service actually failing); gateway/order only show the error rate
	// they inherited from the cascade. This is the trace-independent signal
	// available today (rca's temporal-precedence bonus additionally needs
	// span timing data via Incident.TraceIDs, which is a separate, deeper
	// pre-existing gap -- also affects blast.Analyzer -- out of scope here;
	// this test only proves the unmodified RCA engine correctly uses
	// whatever evidence correlation hands it).
	addErrorRateEvidence(evStore, gateway, "atlas-gateway")
	addErrorRateEvidence(evStore, order, "atlas-order-service")
	addErrorRateEvidence(evStore, payment, "atlas-payment-service")
	addLatencyEvidence(evStore, payment, "atlas-payment-service")

	incidents := []*incidentmodel.Incident{gateway, order, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(3*time.Second))

	primary := payment // the causal rule must have picked payment; verified by other tests too
	if primary.PrimaryIncidentID != payment.IncidentID {
		t.Fatalf("setup assumption broken: expected payment to be primary")
	}

	rcaEngine.Analyze(primary)

	if primary.RCA == nil {
		t.Fatal("expected RCA to produce a result")
	}
	if primary.RCA.Service != "atlas-payment-service" {
		t.Fatalf("expected unmodified RCA to correctly identify atlas-payment-service as root cause using the merged cascade evidence, got %q (reason: %s)", primary.RCA.Service, primary.DetectionReason)
	}
}

func TestCorrelate_ParallelFailureIntegration_RCAReturnsAmbiguous(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	g.AddDependency("atlas-order-service", "atlas-payment-service", 10, true, "5xx")
	g.AddDependency("atlas-order-service", "atlas-inventory-service", 10, true, "5xx")

	rcaEngine, evStore, _ := newRCAEngine(g)

	base := time.Now()
	order := newIncident("atlas-order-service", base.Add(1*time.Second))
	inventory := newIncident("atlas-inventory-service", base)
	payment := newIncident("atlas-payment-service", base)

	// identical evidence strength for both candidates -> scores land within
	// rca's existing ambiguity margin
	addErrorRateEvidence(evStore, inventory, "atlas-inventory-service")
	addErrorRateEvidence(evStore, payment, "atlas-payment-service")

	incidents := []*incidentmodel.Incident{order, inventory, payment}
	NewCorrelator(20).Correlate(incidents, g, base.Add(2*time.Second))

	// whichever of inventory/payment the tie-break picked is primary
	var primary *incidentmodel.Incident
	for _, inc := range incidents {
		if inc.PrimaryIncidentID == inc.IncidentID {
			primary = inc
		}
	}
	if primary == nil {
		t.Fatal("expected exactly one primary in the group")
	}

	rcaEngine.Analyze(primary)

	if primary.RCA == nil {
		t.Fatal("expected RCA to produce a result")
	}
	if primary.RCA.Service != "AMBIGUOUS" {
		t.Fatalf("expected unmodified RCA to correctly report AMBIGUOUS for two equally-scored independent candidates, got %q", primary.RCA.Service)
	}
}
