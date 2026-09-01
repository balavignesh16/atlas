package registry

import (
	"testing"
	"time"
)

func TestRecord_StrongerEvidenceReplacesWeaker(t *testing.T) {
	store := newTestStore(t)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if err := store.Record(Evidence{ServiceName: "payments", Source: ProvenanceDeclared, ObservedAt: t1}); err != nil {
		t.Fatalf("Record (DECLARED): %v", err)
	}
	if err := store.Record(Evidence{ServiceName: "payments", Source: ProvenanceObservedTelemetry, ObservedAt: t2}); err != nil {
		t.Fatalf("Record (OBSERVED_TELEMETRY): %v", err)
	}

	svc, _, _ := store.Get("payments")
	if svc.Provenance != ProvenanceObservedTelemetry {
		t.Fatalf("expected real telemetry to become authoritative over a prior DECLARED entry, got %s", svc.Provenance)
	}
}

func TestRecord_WeakerEvidenceDoesNotOverwriteStronger(t *testing.T) {
	store := newTestStore(t)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if err := store.Record(Evidence{ServiceName: "payments", Source: ProvenanceObservedTelemetry, ObservedAt: t1}); err != nil {
		t.Fatalf("Record (OBSERVED_TELEMETRY): %v", err)
	}
	if err := store.Record(Evidence{ServiceName: "payments", Source: ProvenanceDeclared, ObservedAt: t2}); err != nil {
		t.Fatalf("Record (DECLARED): %v", err)
	}

	svc, _, _ := store.Get("payments")
	if svc.Provenance != ProvenanceObservedTelemetry {
		t.Fatalf("expected a later but weaker DECLARED sighting to NOT downgrade the recorded identity, got %s", svc.Provenance)
	}
	// But existence/recency still advances -- a weak source confirming the
	// service is still around is real information, even if it can't change
	// who's authoritative for its identity.
	if !svc.LastObservedAt.Equal(t2) {
		t.Fatalf("expected LastObservedAt to still advance to the weaker evidence's timestamp %v, got %v", t2, svc.LastObservedAt)
	}
}

func TestRecord_EqualPrecedenceEvidence_LatestSourceWins(t *testing.T) {
	store := newTestStore(t)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if err := store.Record(Evidence{ServiceName: "cache", Source: ProvenanceDocker, ObservedAt: t1}); err != nil {
		t.Fatalf("Record (DOCKER): %v", err)
	}
	if err := store.Record(Evidence{ServiceName: "cache", Source: ProvenanceKubernetes, ObservedAt: t2}); err != nil {
		t.Fatalf("Record (KUBERNETES): %v", err)
	}

	svc, _, _ := store.Get("cache")
	if svc.Provenance != ProvenanceKubernetes {
		t.Fatalf("expected the later, equal-precedence KUBERNETES evidence to become authoritative, got %s", svc.Provenance)
	}
}

func TestRecord_StaleEvidence_DoesNotRegressLastObservedAt(t *testing.T) {
	store := newTestStore(t)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	stale := t1.Add(-24 * time.Hour) // arrives late, older than anything recorded

	if err := store.Record(Evidence{ServiceName: "orders", Source: ProvenanceObservedTelemetry, ObservedAt: t2}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := store.Record(Evidence{ServiceName: "orders", Source: ProvenanceObservedTelemetry, ObservedAt: stale}); err != nil {
		t.Fatalf("Record (stale): %v", err)
	}

	svc, _, _ := store.Get("orders")
	if !svc.LastObservedAt.Equal(t2) {
		t.Fatalf("a stale (older) evidence item must not move LastObservedAt backward: expected %v, got %v", t2, svc.LastObservedAt)
	}
}

func TestRecord_ConflictingEvidence_DeterministicRegardlessOfInsertionOrder(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(30 * time.Minute)
	t3 := t1.Add(90 * time.Minute)

	evidenceA := Evidence{ServiceName: "gateway", Source: ProvenanceObservedTelemetry, ObservedAt: t2}
	evidenceB := Evidence{ServiceName: "gateway", Source: ProvenanceDeclared, ObservedAt: t3} // later, but weaker
	evidenceC := Evidence{ServiceName: "gateway", Source: ProvenanceDocker, ObservedAt: t1}   // earliest, mid-strength

	orderings := [][]Evidence{
		{evidenceA, evidenceB, evidenceC},
		{evidenceC, evidenceB, evidenceA},
		{evidenceB, evidenceA, evidenceC},
		{evidenceC, evidenceA, evidenceB},
	}

	var results []Service
	for _, ordering := range orderings {
		store := newTestStore(t)
		for _, e := range ordering {
			if err := store.Record(e); err != nil {
				t.Fatalf("Record: %v", err)
			}
		}
		svc, ok, err := store.Get("gateway")
		if err != nil || !ok {
			t.Fatalf("Get: ok=%v err=%v", ok, err)
		}
		results = append(results, svc)
	}

	for i := 1; i < len(results); i++ {
		if results[i].Provenance != results[0].Provenance {
			t.Fatalf("Provenance depends on insertion order: got %s at index 0 but %s at index %d", results[0].Provenance, results[i].Provenance, i)
		}
		if !results[i].LastObservedAt.Equal(results[0].LastObservedAt) {
			t.Fatalf("LastObservedAt depends on insertion order: got %v at index 0 but %v at index %d", results[0].LastObservedAt, results[i].LastObservedAt, i)
		}
		if !results[i].FirstObservedAt.Equal(results[0].FirstObservedAt) {
			t.Fatalf("FirstObservedAt depends on insertion order: got %v at index 0 but %v at index %d", results[0].FirstObservedAt, results[i].FirstObservedAt, i)
		}
	}
	// The real evidence (OBSERVED_TELEMETRY) must win regardless of order,
	// since it has the highest precedence of the three.
	if results[0].Provenance != ProvenanceObservedTelemetry {
		t.Fatalf("expected OBSERVED_TELEMETRY to be authoritative regardless of insertion order, got %s", results[0].Provenance)
	}
	// FirstObservedAt must be the earliest of the three (t1), and
	// LastObservedAt the latest (t3), regardless of order.
	if !results[0].FirstObservedAt.Equal(t1) {
		t.Errorf("expected FirstObservedAt = %v (earliest evidence), got %v", t1, results[0].FirstObservedAt)
	}
	if !results[0].LastObservedAt.Equal(t3) {
		t.Errorf("expected LastObservedAt = %v (latest evidence), got %v", t3, results[0].LastObservedAt)
	}
}

func TestRecord_NonTelemetryEvidence_DoesNotSetLastTelemetryAt(t *testing.T) {
	store := newTestStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := store.Record(Evidence{ServiceName: "declared-only", Source: ProvenanceDeclared, ObservedAt: at}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	svc, _, _ := store.Get("declared-only")
	if svc.LastTelemetryAt != nil {
		t.Fatalf("a service known only via DECLARED evidence must have no LastTelemetryAt, got %v", svc.LastTelemetryAt)
	}
}

func TestRecord_TelemetryEvidenceAfterNonTelemetry_SetsLastTelemetryAt(t *testing.T) {
	store := newTestStore(t)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if err := store.Record(Evidence{ServiceName: "svc", Source: ProvenanceDeclared, ObservedAt: t1}); err != nil {
		t.Fatalf("Record (DECLARED): %v", err)
	}
	if err := store.Record(Evidence{ServiceName: "svc", Source: ProvenanceObservedTelemetry, ObservedAt: t2}); err != nil {
		t.Fatalf("Record (OBSERVED_TELEMETRY): %v", err)
	}

	svc, _, _ := store.Get("svc")
	if svc.LastTelemetryAt == nil || !svc.LastTelemetryAt.Equal(t2) {
		t.Fatalf("expected LastTelemetryAt = %v once real telemetry arrives, got %v", t2, svc.LastTelemetryAt)
	}
}

func TestRecord_UnknownSource_Rejected(t *testing.T) {
	store := newTestStore(t)
	err := store.Record(Evidence{ServiceName: "svc", Source: Provenance("MADE_UP"), ObservedAt: time.Now()})
	if err == nil {
		t.Fatal("expected an error for an unrecognized evidence source")
	}
}

func TestObserve_StillWorksIdenticallyThroughRecord(t *testing.T) {
	store := newTestStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("legacy-caller", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	svc, ok, _ := store.Get("legacy-caller")
	if !ok || svc.Provenance != ProvenanceObservedTelemetry || svc.Status != StatusActive {
		t.Fatalf("Observe must behave exactly as it did in Phase 7B: got ok=%v svc=%+v", ok, svc)
	}
}
