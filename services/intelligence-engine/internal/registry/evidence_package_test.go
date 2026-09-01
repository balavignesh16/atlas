package registry

import (
	"testing"
	"time"
)

func TestBuildEvidencePackage_UnknownService_ReturnsNotOk(t *testing.T) {
	store := newTestStore(t)
	_, ok, err := store.BuildEvidencePackage("never-seen", nil, nil, time.Now())
	if err != nil {
		t.Fatalf("BuildEvidencePackage: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for a service the registry has never observed")
	}
}

func TestBuildEvidencePackage_KnownService_ReflectsRealRegistryState(t *testing.T) {
	store := newTestStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("checkout-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	generatedAt := at.Add(time.Minute)
	pkg, ok, err := store.BuildEvidencePackage("checkout-service", []string{"payment-service"}, []string{"1 open incident"}, generatedAt)
	if err != nil {
		t.Fatalf("BuildEvidencePackage: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for a known service")
	}

	if pkg.ServiceName != "checkout-service" {
		t.Errorf("ServiceName = %q, want checkout-service", pkg.ServiceName)
	}
	if pkg.LifecycleStatus != StatusActive {
		t.Errorf("LifecycleStatus = %q, want ACTIVE", pkg.LifecycleStatus)
	}
	if pkg.Provenance != ProvenanceObservedTelemetry {
		t.Errorf("Provenance = %q, want OBSERVED_TELEMETRY", pkg.Provenance)
	}
	if pkg.Confidence != ConfidenceObserved {
		t.Errorf("Confidence = %q, want OBSERVED", pkg.Confidence)
	}
	if !pkg.FirstObservedAt.Equal(at) || !pkg.LastObservedAt.Equal(at) {
		t.Errorf("expected timestamps to reflect the real registry record, got first=%v last=%v", pkg.FirstObservedAt, pkg.LastObservedAt)
	}
	if len(pkg.Dependencies) != 1 || pkg.Dependencies[0] != "payment-service" {
		t.Errorf("expected Dependencies to be exactly the caller-supplied slice, got %v", pkg.Dependencies)
	}
	if len(pkg.IncidentFacts) != 1 || pkg.IncidentFacts[0] != "1 open incident" {
		t.Errorf("expected IncidentFacts to be exactly the caller-supplied slice, got %v", pkg.IncidentFacts)
	}
	if !pkg.GeneratedAt.Equal(generatedAt) {
		t.Errorf("GeneratedAt = %v, want the caller-supplied %v", pkg.GeneratedAt, generatedAt)
	}
}

func TestBuildEvidencePackage_NoDependenciesOrIncidentsSupplied_StaysNilNotFabricated(t *testing.T) {
	store := newTestStore(t)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Observe("isolated-service", at); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	pkg, ok, err := store.BuildEvidencePackage("isolated-service", nil, nil, at)
	if err != nil || !ok {
		t.Fatalf("BuildEvidencePackage: ok=%v err=%v", ok, err)
	}
	if pkg.Dependencies != nil {
		t.Errorf("expected nil Dependencies when the caller supplied none, got %v", pkg.Dependencies)
	}
	if pkg.IncidentFacts != nil {
		t.Errorf("expected nil IncidentFacts when the caller supplied none, got %v", pkg.IncidentFacts)
	}
}

func TestConfidenceFor_EveryDeclaredProvenanceHasAConfidence(t *testing.T) {
	cases := map[Provenance]Confidence{
		ProvenanceObservedTelemetry: ConfidenceObserved,
		ProvenanceDocker:            ConfidenceObserved,
		ProvenanceKubernetes:        ConfidenceObserved,
		ProvenanceDeclared:          ConfidenceDeclared,
		ProvenanceConfig:            ConfidenceDeclared,
		ProvenanceInferred:          ConfidenceInferred,
	}
	for provenance, want := range cases {
		if got := ConfidenceFor(provenance); got != want {
			t.Errorf("ConfidenceFor(%s) = %s, want %s", provenance, got, want)
		}
	}
}
