package registry

import (
	"testing"
	"time"
)

// seedFilterFixture seeds a deliberately mixed fixture:
//   - checkout-service: recent OBSERVED_TELEMETRY -> stays ACTIVE
//   - payment-service: very old OBSERVED_TELEMETRY -> retires
//   - legacy-worker: DECLARED, no telemetry at all -> stays ACTIVE forever
//     under the current lifecycle sweep regardless of elapsed time (see
//     EvaluateLifecycle's own doc comment: it keys strictly on
//     LastTelemetryAt, which a DECLARED-only record never has).
func seedFilterFixture(t *testing.T, store *Store) {
	t.Helper()
	recentAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	oldAt := recentAt.Add(-365 * 24 * time.Hour)

	if err := store.Record(Evidence{ServiceName: "checkout-service", Source: ProvenanceObservedTelemetry, ObservedAt: recentAt}); err != nil {
		t.Fatalf("seed checkout-service: %v", err)
	}
	if err := store.Record(Evidence{ServiceName: "payment-service", Source: ProvenanceObservedTelemetry, ObservedAt: oldAt}); err != nil {
		t.Fatalf("seed payment-service: %v", err)
	}
	if err := store.Record(Evidence{ServiceName: "legacy-worker", Source: ProvenanceDeclared, ObservedAt: recentAt}); err != nil {
		t.Fatalf("seed legacy-worker: %v", err)
	}
	if err := store.EvaluateLifecycle(recentAt, time.Hour, 2*time.Hour); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}
}

func TestList_FilterByStatus(t *testing.T) {
	store := newTestStore(t)
	seedFilterFixture(t, store)

	retired := StatusRetired
	results, err := store.List(ListFilter{Status: &retired})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].Name != "payment-service" {
		t.Fatalf("expected only payment-service (old telemetry) for status=RETIRED, got %+v", results)
	}
}

func TestList_FilterBySource(t *testing.T) {
	store := newTestStore(t)
	seedFilterFixture(t, store)

	declared := ProvenanceDeclared
	results, err := store.List(ListFilter{Source: &declared})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].Name != "legacy-worker" {
		t.Fatalf("expected only legacy-worker for source=DECLARED, got %+v", results)
	}
}

func TestList_FilterByQuery_CaseInsensitiveSubstring(t *testing.T) {
	store := newTestStore(t)
	seedFilterFixture(t, store)

	results, err := store.List(ListFilter{Query: "SERVICE"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 services matching \"SERVICE\" case-insensitively, got %d: %+v", len(results), results)
	}
}

func TestList_FilterByQuery_NoMatch_ReturnsEmptyNotError(t *testing.T) {
	store := newTestStore(t)
	seedFilterFixture(t, store)

	results, err := store.List(ListFilter{Query: "does-not-exist-anywhere"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for a non-matching query, got %d", len(results))
	}
}

func TestList_CombinedFilters(t *testing.T) {
	store := newTestStore(t)
	seedFilterFixture(t, store)

	active := StatusActive
	results, err := store.List(ListFilter{Status: &active, Query: "checkout"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].Name != "checkout-service" {
		t.Fatalf("expected only checkout-service, got %+v", results)
	}
}

func TestList_QueryWithLikeMetacharacters_TreatedLiterally(t *testing.T) {
	store := newTestStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("100%-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := store.Observe("other-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	// A literal "%" in the query must not act as a SQL wildcard matching
	// everything -- it should only match names that actually contain "%".
	results, err := store.List(ListFilter{Query: "100%"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(results) != 1 || results[0].Name != "100%-service" {
		t.Fatalf("expected the literal '%%' to match only the service containing it, got %+v", results)
	}
}

func TestList_DeterministicOrderingAcrossFilterCombinations(t *testing.T) {
	store := newTestStore(t)
	seedFilterFixture(t, store)

	first, err := store.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	second, err := store.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("repeated identical List calls returned different lengths: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name != second[i].Name {
			t.Fatalf("List ordering is not deterministic across repeated calls at index %d: %q vs %q", i, first[i].Name, second[i].Name)
		}
	}
}
