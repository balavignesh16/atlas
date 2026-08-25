package incidentmanager

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentdetector"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/google/uuid"
)

// newTestCausalAnalyzer builds a CausalAnalyzer from incidentdetector's real
// DefaultConfig, exactly as main.go wires it -- proving these tests exercise
// the same failing-dependency definition M2.4 uses, not a reinvented one.
func newTestCausalAnalyzer() *CausalAnalyzer {
	cfg := incidentdetector.DefaultConfig()
	return NewCausalAnalyzer(cfg.MinObservations, cfg.DependencyErrorRateThreshold)
}

// seedEdge adds `calls` observations to source->target, `errors` of which
// are failures, via the real graph.DependencyGraph.AddDependency path (the
// same one incidentdetector/correlation use), so CallCount/ErrorCount are
// genuine, not hand-constructed.
func seedEdge(g *graph.DependencyGraph, source, target string, calls, errors int) {
	for i := 0; i < calls; i++ {
		isError := i < errors
		g.AddDependency(source, target, 10, isError, "OK")
	}
}

func TestIsFailingEdge_MatchesM24Semantics(t *testing.T) {
	c := NewCausalAnalyzer(10, 0.20) // exact incidentdetector.DefaultConfig() values

	cases := []struct {
		name  string
		calls int
		errs  int
		want  bool
	}{
		{"below MinObservations, 100% errors -> not failing (volume gate)", 5, 5, false},
		{"exactly at MinObservations, exactly at threshold (20%) -> not failing (strict >)", 10, 2, false},
		{"exactly at MinObservations, just above threshold (30%) -> failing", 10, 3, true},
		{"well above MinObservations, 0% errors -> not failing", 20, 0, false},
		{"well above MinObservations, well above threshold -> failing", 20, 15, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := graph.NewDependencyGraph(3600)
			seedEdge(g, "caller", "callee", tc.calls, tc.errs)
			edges := g.GetEdges()
			if len(edges) != 1 {
				t.Fatalf("expected exactly 1 edge, got %d", len(edges))
			}
			if got := c.isFailingEdge(edges[0]); got != tc.want {
				t.Errorf("isFailingEdge(calls=%d, errors=%d) = %v, want %v", tc.calls, tc.errs, got, tc.want)
			}
		})
	}
}

func TestResolveCausalSinksFor_LinearChainResolvesToDeepestSink(t *testing.T) {
	c := newTestCausalAnalyzer()
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "gateway", "order", 20, 15)
	seedEdge(g, "order", "payment", 20, 15)
	edges := g.GetEdges()
	group := map[string]bool{"gateway": true, "order": true, "payment": true}

	sinks := c.ResolveCausalSinksFor("gateway", group, edges)
	if len(sinks) != 1 || !sinks["payment"] {
		t.Fatalf("expected gateway to resolve to {payment}, got %v", sinks)
	}
}

func TestResolveCausalSinksFor_BranchingResolvesToBothSinks(t *testing.T) {
	c := newTestCausalAnalyzer()
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "order", "payment", 20, 15)
	seedEdge(g, "order", "inventory", 20, 15)
	edges := g.GetEdges()
	group := map[string]bool{"order": true, "payment": true, "inventory": true}

	sinks := c.ResolveCausalSinksFor("order", group, edges)
	if len(sinks) != 2 || !sinks["payment"] || !sinks["inventory"] {
		t.Fatalf("expected order to resolve to {payment, inventory}, got %v", sinks)
	}
}

func TestResolveCausalSinksFor_NoFailingEdgeIsItsOwnSink(t *testing.T) {
	c := newTestCausalAnalyzer()
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "payment", "external-gateway", 20, 0) // healthy call, not in group anyway
	edges := g.GetEdges()
	group := map[string]bool{"payment": true}

	sinks := c.ResolveCausalSinksFor("payment", group, edges)
	if len(sinks) != 1 || !sinks["payment"] {
		t.Fatalf("expected a service with no failing outgoing edge to be its own sink, got %v", sinks)
	}
}

func TestResolveCausalSinksFor_TargetOutsideGroupIsNotFollowed(t *testing.T) {
	c := newTestCausalAnalyzer()
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "order", "payment", 20, 15) // failing, but payment is NOT in this group
	edges := g.GetEdges()
	group := map[string]bool{"order": true} // payment deliberately excluded (incomplete graph / not currently correlated)

	sinks := c.ResolveCausalSinksFor("order", group, edges)
	if len(sinks) != 1 || !sinks["order"] {
		t.Fatalf("expected order to remain its own sink when its failing target is outside the group, got %v -- never invent a relationship the group doesn't show", sinks)
	}
}

func TestResolveCausalSinksFor_CycleResolvesToEmptySafely(t *testing.T) {
	c := newTestCausalAnalyzer()
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "a", "b", 20, 15)
	seedEdge(g, "b", "a", 20, 15)
	edges := g.GetEdges()
	group := map[string]bool{"a": true, "b": true}

	sinksA := c.ResolveCausalSinksFor("a", group, edges)
	sinksB := c.ResolveCausalSinksFor("b", group, edges)
	if len(sinksA) != 0 {
		t.Fatalf("expected a cycle to resolve to no safe sink for a, got %v", sinksA)
	}
	if len(sinksB) != 0 {
		t.Fatalf("expected a cycle to resolve to no safe sink for b, got %v", sinksB)
	}
}

// --- ApplyCausalAttribution: evidence-pool-level tests ---

func newGroupedIncident(id, service, groupID, primaryID string, evidenceIDs []string, startedAt time.Time) *incidentmodel.Incident {
	return &incidentmodel.Incident{
		IncidentID:         id,
		Status:             incidentmodel.StatusOpen,
		StartedAt:          startedAt,
		LastUpdatedAt:      startedAt,
		RootService:        service,
		AffectedServices:   []string{service},
		EvidenceIDs:        evidenceIDs,
		CorrelationGroupID: groupID,
		PrimaryIncidentID:  primaryID,
	}
}

func addDependencyErrorEv(evStore *evidence.Store, service string) string {
	ev := evidence.Evidence{
		EvidenceID:  service + "-dep-err-" + uuid.New().String(),
		Type:        evidence.EvidenceTypeDependencyError,
		Timestamp:   time.Now(),
		Service:     service,
		Description: service + " dependency failure",
	}
	evStore.Add(ev)
	return ev.EvidenceID
}

func TestApplyCausalAttribution_LinearRedirectsCallersEvidenceToTrueRoot(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-gateway", "atlas-order-service", 20, 15)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)
	evStore := evidence.NewStore()

	gwDepEv := addDependencyErrorEv(evStore, "atlas-gateway")
	orderDepEv := addDependencyErrorEv(evStore, "atlas-order-service")

	now := time.Now()
	// primary is atlas-payment-service (as Correlator would have already
	// selected); its EvidenceIDs already carries the group's merged pool.
	primary := newGroupedIncident("payment-inc", "atlas-payment-service", "group-1", "payment-inc",
		[]string{gwDepEv, orderDepEv}, now)
	gateway := newGroupedIncident("gw-inc", "atlas-gateway", "group-1", "payment-inc", nil, now)
	order := newGroupedIncident("order-inc", "atlas-order-service", "group-1", "payment-inc", nil, now)

	c := newTestCausalAnalyzer()
	c.ApplyCausalAttribution([]*incidentmodel.Incident{gateway, order, primary}, g, evStore)

	if len(primary.EvidenceIDs) != 1 {
		t.Fatalf("expected exactly 1 evidence ID after redirection (both suppressed callers collapse to one payment-attributed entry), got %d: %v", len(primary.EvidenceIDs), primary.EvidenceIDs)
	}
	redirected, ok := evStore.Get(primary.EvidenceIDs[0])
	if !ok {
		t.Fatal("redirected evidence not found in store")
	}
	if redirected.Service != "atlas-payment-service" {
		t.Fatalf("expected redirected evidence attributed to atlas-payment-service, got %s", redirected.Service)
	}
	if redirected.Type != evidence.EvidenceTypeDependencyError {
		t.Fatalf("expected redirected evidence to stay DEPENDENCY_ERROR type (so rca.Engine's existing branch scores it unmodified), got %s", redirected.Type)
	}
}

func TestApplyCausalAttribution_DeduplicatesRedirectionForSameSink(t *testing.T) {
	// Two DIFFERENT callers (gateway, order) both redirect to the SAME sink
	// (payment) once order's own evidence is also traced through. This
	// proves one destination service never receives more than one new
	// redirected evidence entry, regardless of how many callers point to it.
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-gateway", "atlas-order-service", 20, 15)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)
	evStore := evidence.NewStore()

	gwDepEv := addDependencyErrorEv(evStore, "atlas-gateway")
	orderDepEv := addDependencyErrorEv(evStore, "atlas-order-service")

	now := time.Now()
	primary := newGroupedIncident("payment-inc", "atlas-payment-service", "group-1", "payment-inc",
		[]string{gwDepEv, orderDepEv}, now)
	gateway := newGroupedIncident("gw-inc", "atlas-gateway", "group-1", "payment-inc", nil, now)
	order := newGroupedIncident("order-inc", "atlas-order-service", "group-1", "payment-inc", nil, now)

	c := newTestCausalAnalyzer()
	c.ApplyCausalAttribution([]*incidentmodel.Incident{gateway, order, primary}, g, evStore)

	paymentAttributed := 0
	for _, id := range primary.EvidenceIDs {
		ev, ok := evStore.Get(id)
		if ok && ev.Service == "atlas-payment-service" && ev.Type == evidence.EvidenceTypeDependencyError {
			paymentAttributed++
		}
	}
	if paymentAttributed != 1 {
		t.Fatalf("expected exactly 1 DEPENDENCY_ERROR evidence entry attributed to atlas-payment-service (no double-counting), got %d", paymentAttributed)
	}
}

func TestApplyCausalAttribution_SharedCaller_BothSinksCreditedCallerNot(t *testing.T) {
	// Order -> Payment, Order -> Inventory, both failing. Requirement: Payment
	// and Inventory each receive their own redirected evidence; Order does
	// not retain any DEPENDENCY_ERROR credit.
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)
	seedEdge(g, "atlas-order-service", "atlas-inventory-service", 20, 15)
	evStore := evidence.NewStore()

	orderToPaymentEv := addDependencyErrorEv(evStore, "atlas-order-service")
	// A second, distinct evidence item representing order's OTHER failing
	// dependency (inventory) -- both are legitimately attributed to Order by
	// M2.4 today, since evaluateDependencies emits one signal per edge.
	orderToInventoryEv := addDependencyErrorEv(evStore, "atlas-order-service")

	now := time.Now()
	primary := newGroupedIncident("order-inc", "atlas-order-service", "group-1", "order-inc",
		[]string{orderToPaymentEv, orderToInventoryEv}, now)
	payment := newGroupedIncident("payment-inc", "atlas-payment-service", "group-1", "order-inc", nil, now)
	inventory := newGroupedIncident("inventory-inc", "atlas-inventory-service", "group-1", "order-inc", nil, now)

	c := newTestCausalAnalyzer()
	c.ApplyCausalAttribution([]*incidentmodel.Incident{primary, payment, inventory}, g, evStore)

	sawPayment, sawInventory, sawOrder := false, false, false
	for _, id := range primary.EvidenceIDs {
		ev, ok := evStore.Get(id)
		if !ok || ev.Type != evidence.EvidenceTypeDependencyError {
			continue
		}
		switch ev.Service {
		case "atlas-payment-service":
			sawPayment = true
		case "atlas-inventory-service":
			sawInventory = true
		case "atlas-order-service":
			sawOrder = true
		}
	}
	if !sawPayment {
		t.Error("expected atlas-payment-service to receive redirected DEPENDENCY_ERROR evidence")
	}
	if !sawInventory {
		t.Error("expected atlas-inventory-service to receive redirected DEPENDENCY_ERROR evidence")
	}
	if sawOrder {
		t.Error("expected atlas-order-service to retain NO DEPENDENCY_ERROR evidence -- both its edges were redirected away")
	}
}

func TestApplyCausalAttribution_CycleLeavesEvidenceUnredirected(t *testing.T) {
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "service-a", "service-b", 20, 15)
	seedEdge(g, "service-b", "service-a", 20, 15)
	evStore := evidence.NewStore()

	aDepEv := addDependencyErrorEv(evStore, "service-a")

	now := time.Now()
	primary := newGroupedIncident("a-inc", "service-a", "group-1", "a-inc", []string{aDepEv}, now)
	b := newGroupedIncident("b-inc", "service-b", "group-1", "a-inc", nil, now)

	c := newTestCausalAnalyzer()
	c.ApplyCausalAttribution([]*incidentmodel.Incident{primary, b}, g, evStore)

	if len(primary.EvidenceIDs) != 1 || primary.EvidenceIDs[0] != aDepEv {
		t.Fatalf("expected a cycle to leave evidence exactly as originally attributed (safe fallback), got %v", primary.EvidenceIDs)
	}
}

func TestApplyCausalAttribution_FailsOpenOnNilGraph(t *testing.T) {
	evStore := evidence.NewStore()
	depEv := addDependencyErrorEv(evStore, "atlas-order-service")
	now := time.Now()
	primary := newGroupedIncident("order-inc", "atlas-order-service", "group-1", "order-inc", []string{depEv}, now)

	c := newTestCausalAnalyzer()
	c.ApplyCausalAttribution([]*incidentmodel.Incident{primary}, nil, evStore)

	if len(primary.EvidenceIDs) != 1 || primary.EvidenceIDs[0] != depEv {
		t.Fatalf("expected fail-open (no change) when depGraph is nil, got %v", primary.EvidenceIDs)
	}
}

func TestApplyCausalAttribution_TargetOutsideGroupLeavesEvidenceAttributed(t *testing.T) {
	// Order's dependency failure points at a service that isn't part of this
	// correlated incident (e.g. an external/unmonitored dependency, or
	// simply not currently correlated). Nothing to redirect to -- must not
	// invent a relationship the group doesn't show.
	g := graph.NewDependencyGraph(3600)
	seedEdge(g, "atlas-order-service", "atlas-payment-service", 20, 15)
	evStore := evidence.NewStore()
	orderDepEv := addDependencyErrorEv(evStore, "atlas-order-service")

	now := time.Now()
	// Only order-service is in this group; payment-service is deliberately
	// NOT a member (e.g. its own incident hasn't been correlated in yet).
	primary := newGroupedIncident("order-inc", "atlas-order-service", "group-1", "order-inc", []string{orderDepEv}, now)

	c := newTestCausalAnalyzer()
	c.ApplyCausalAttribution([]*incidentmodel.Incident{primary}, g, evStore)

	if len(primary.EvidenceIDs) != 1 || primary.EvidenceIDs[0] != orderDepEv {
		t.Fatalf("expected evidence to remain attributed to order-service when its failing target isn't in the group, got %v", primary.EvidenceIDs)
	}
}
