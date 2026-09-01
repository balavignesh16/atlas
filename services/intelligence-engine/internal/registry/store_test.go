package registry

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestObserve_FirstSighting_CreatesActiveObservedTelemetryRecord(t *testing.T) {
	store := newTestStore(t)
	at := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if err := store.Observe("checkout-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	svc, ok, err := store.Get("checkout-service")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("expected service to exist after Observe")
	}
	if svc.Name != "checkout-service" {
		t.Errorf("Name = %q, want %q", svc.Name, "checkout-service")
	}
	if svc.DisplayName != "checkout-service" {
		t.Errorf("DisplayName = %q, want it to equal Name (no other evidence exists)", svc.DisplayName)
	}
	if svc.Provenance != ProvenanceObservedTelemetry {
		t.Errorf("Provenance = %q, want %q", svc.Provenance, ProvenanceObservedTelemetry)
	}
	if svc.Status != StatusActive {
		t.Errorf("Status = %q, want %q", svc.Status, StatusActive)
	}
	if !svc.FirstObservedAt.Equal(at) || !svc.LastObservedAt.Equal(at) || !svc.CreatedAt.Equal(at) || !svc.UpdatedAt.Equal(at) {
		t.Errorf("expected all timestamps to equal %v on first sighting, got %+v", at, svc)
	}
	if svc.LastTelemetryAt == nil || !svc.LastTelemetryAt.Equal(at) {
		t.Errorf("expected LastTelemetryAt = %v, got %v", at, svc.LastTelemetryAt)
	}
}

func TestObserve_DuplicateSighting_UpdatesWithoutCreatingASecondRecord(t *testing.T) {
	store := newTestStore(t)
	first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	second := first.Add(10 * time.Minute)

	if err := store.Observe("checkout-service", first); err != nil {
		t.Fatalf("Observe (first): %v", err)
	}
	if err := store.Observe("checkout-service", second); err != nil {
		t.Fatalf("Observe (second): %v", err)
	}

	all, err := store.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected exactly 1 service after 2 sightings of the same name, got %d: %+v", len(all), all)
	}

	svc := all[0]
	if !svc.FirstObservedAt.Equal(first) {
		t.Errorf("FirstObservedAt should be preserved as %v, got %v", first, svc.FirstObservedAt)
	}
	if !svc.CreatedAt.Equal(first) {
		t.Errorf("CreatedAt should be preserved as %v, got %v", first, svc.CreatedAt)
	}
	if !svc.LastObservedAt.Equal(second) {
		t.Errorf("LastObservedAt should advance to %v, got %v", second, svc.LastObservedAt)
	}
	if svc.LastTelemetryAt == nil || !svc.LastTelemetryAt.Equal(second) {
		t.Errorf("LastTelemetryAt should advance to %v, got %v", second, svc.LastTelemetryAt)
	}
}

func TestObserve_RejectsEmptyName(t *testing.T) {
	store := newTestStore(t)
	if err := store.Observe("", time.Now()); err == nil {
		t.Fatal("expected an error for an empty service name")
	}
}

func TestGet_UnknownService_ReturnsNotOkWithoutError(t *testing.T) {
	store := newTestStore(t)
	svc, ok, err := store.Get("never-seen")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false for an unknown service, got %+v", svc)
	}
}

func TestList_ReturnsAllServicesAlphabetically(t *testing.T) {
	store := newTestStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"zeta-service", "alpha-service", "mid-service"} {
		if err := store.Observe(name, at); err != nil {
			t.Fatalf("Observe(%s): %v", name, err)
		}
	}

	all, err := store.List(ListFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := []string{all[0].Name, all[1].Name, all[2].Name}
	want := []string{"alpha-service", "mid-service", "zeta-service"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List order = %v, want %v", got, want)
		}
	}
}

func TestEvaluateLifecycle_TransitionsActiveToStaleAfterStaleWindow(t *testing.T) {
	store := newTestStore(t)
	seenAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("quiet-service", seenAt); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	staleAfter := 30 * time.Minute
	retireAfter := 24 * time.Hour

	// Not yet past the stale window: still ACTIVE.
	if err := store.EvaluateLifecycle(seenAt.Add(10*time.Minute), staleAfter, retireAfter); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}
	svc, _, _ := store.Get("quiet-service")
	if svc.Status != StatusActive {
		t.Fatalf("expected still ACTIVE before the stale window elapses, got %s", svc.Status)
	}

	// Past the stale window: STALE.
	if err := store.EvaluateLifecycle(seenAt.Add(45*time.Minute), staleAfter, retireAfter); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}
	svc, _, _ = store.Get("quiet-service")
	if svc.Status != StatusStale {
		t.Fatalf("expected STALE after the stale window elapses, got %s", svc.Status)
	}
}

func TestEvaluateLifecycle_TransitionsStaleToRetiredAfterRetireWindow(t *testing.T) {
	store := newTestStore(t)
	seenAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("abandoned-service", seenAt); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	staleAfter := 30 * time.Minute
	retireAfter := 2 * time.Hour

	if err := store.EvaluateLifecycle(seenAt.Add(1*time.Hour), staleAfter, retireAfter); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}
	svc, _, _ := store.Get("abandoned-service")
	if svc.Status != StatusStale {
		t.Fatalf("expected STALE at 1h, got %s", svc.Status)
	}

	if err := store.EvaluateLifecycle(seenAt.Add(3*time.Hour), staleAfter, retireAfter); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}
	svc, _, _ = store.Get("abandoned-service")
	if svc.Status != StatusRetired {
		t.Fatalf("expected RETIRED at 3h, got %s", svc.Status)
	}
}

func TestEvaluateLifecycle_NeverDeletesARetiredRecord(t *testing.T) {
	store := newTestStore(t)
	seenAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("long-gone-service", seenAt); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := store.EvaluateLifecycle(seenAt.Add(365*24*time.Hour), time.Hour, 2*time.Hour); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}

	svc, ok, err := store.Get("long-gone-service")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("a RETIRED service's identity must never be deleted from the registry")
	}
	if svc.Status != StatusRetired {
		t.Fatalf("expected RETIRED, got %s", svc.Status)
	}
	if !svc.FirstObservedAt.Equal(seenAt) {
		t.Errorf("expected FirstObservedAt to survive retirement unchanged, got %v", svc.FirstObservedAt)
	}
}

func TestObserve_ReactivatesARetiredService(t *testing.T) {
	store := newTestStore(t)
	seenAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("comeback-service", seenAt); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := store.EvaluateLifecycle(seenAt.Add(365*24*time.Hour), time.Hour, 2*time.Hour); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}
	svc, _, _ := store.Get("comeback-service")
	if svc.Status != StatusRetired {
		t.Fatalf("setup: expected RETIRED before the comeback, got %s", svc.Status)
	}

	backAt := seenAt.Add(400 * 24 * time.Hour)
	if err := store.Observe("comeback-service", backAt); err != nil {
		t.Fatalf("Observe (comeback): %v", err)
	}

	svc, _, _ = store.Get("comeback-service")
	if svc.Status != StatusActive {
		t.Fatalf("a service that resumes emitting telemetry must become ACTIVE again, got %s", svc.Status)
	}
	if !svc.FirstObservedAt.Equal(seenAt) {
		t.Errorf("FirstObservedAt must still reflect the original sighting, got %v", svc.FirstObservedAt)
	}
}

func TestEvaluateLifecycle_DoesNotAffectAServiceWithRecentTelemetry(t *testing.T) {
	store := newTestStore(t)
	seenAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("busy-service", seenAt); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := store.EvaluateLifecycle(seenAt.Add(time.Second), time.Hour, 2*time.Hour); err != nil {
		t.Fatalf("EvaluateLifecycle: %v", err)
	}
	svc, _, _ := store.Get("busy-service")
	if svc.Status != StatusActive {
		t.Fatalf("expected ACTIVE immediately after observation, got %s", svc.Status)
	}
}

func TestStore_SurvivesRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "registry-restart-test.db")
	seenAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	first, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore (first open): %v", err)
	}
	if err := first.Observe("durable-service", seenAt); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Simulates an intelligence-engine process restart: a brand new Store
	// pointed at the same on-disk file.
	second, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore (reopen after restart): %v", err)
	}
	defer second.Close()

	svc, ok, err := second.Get("durable-service")
	if err != nil {
		t.Fatalf("Get after restart: %v", err)
	}
	if !ok {
		t.Fatal("expected the service to survive a store close/reopen (simulated process restart)")
	}
	if !svc.FirstObservedAt.Equal(seenAt) {
		t.Errorf("expected FirstObservedAt to survive restart unchanged, got %v", svc.FirstObservedAt)
	}
}
