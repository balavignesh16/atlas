package registry

import (
	"testing"
	"time"
)

func TestShouldSupersede_HigherPrecedenceAlwaysWinsRegardlessOfTimestamp(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)

	// A fresh, recent DECLARED entry must not displace an older but
	// stronger OBSERVED_TELEMETRY one.
	if ShouldSupersede(ProvenanceObservedTelemetry, older, ProvenanceDeclared, newer) {
		t.Fatal("weaker evidence (DECLARED) must not supersede stronger evidence (OBSERVED_TELEMETRY), even if newer")
	}
}

func TestShouldSupersede_StrongerEvidenceReplacesWeaker(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)

	if !ShouldSupersede(ProvenanceDeclared, older, ProvenanceObservedTelemetry, newer) {
		t.Fatal("stronger evidence (OBSERVED_TELEMETRY) must supersede weaker evidence (DECLARED)")
	}
	// Even if the stronger evidence is OLDER than the weaker current record.
	if !ShouldSupersede(ProvenanceDeclared, newer, ProvenanceObservedTelemetry, older) {
		t.Fatal("stronger evidence must supersede weaker evidence even when the stronger evidence is chronologically older")
	}
}

func TestShouldSupersede_EqualPrecedence_LatestWins(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)

	if !ShouldSupersede(ProvenanceDocker, t1, ProvenanceKubernetes, t2) {
		t.Fatal("at equal precedence, the later evidence should supersede the earlier")
	}
	if ShouldSupersede(ProvenanceDocker, t2, ProvenanceKubernetes, t1) {
		t.Fatal("at equal precedence, earlier evidence must not supersede later evidence")
	}
}

func TestShouldSupersede_SameSourceDuplicateEvidence_LatestWins(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)

	if !ShouldSupersede(ProvenanceObservedTelemetry, t1, ProvenanceObservedTelemetry, t2) {
		t.Fatal("a later sighting from the same source should supersede an earlier one")
	}
	if ShouldSupersede(ProvenanceObservedTelemetry, t2, ProvenanceObservedTelemetry, t1) {
		t.Fatal("an earlier (stale) sighting from the same source must not supersede a later one")
	}
}

func TestShouldSupersede_FullTie_DeterministicBySourceName(t *testing.T) {
	tie := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Same precedence, same instant: must be a pure function of the two
	// source names, not of argument order or call order.
	first := ShouldSupersede(ProvenanceDocker, tie, ProvenanceKubernetes, tie)
	second := ShouldSupersede(ProvenanceDocker, tie, ProvenanceKubernetes, tie)
	if first != second {
		t.Fatal("ShouldSupersede must be a pure function: identical inputs must always produce identical output")
	}
	// And the two directions must be consistent with each other: exactly
	// one of (A supersedes B) / (B supersedes A) may hold when the sources
	// differ, never both, never neither.
	aOverB := ShouldSupersede(ProvenanceDocker, tie, ProvenanceKubernetes, tie)
	bOverA := ShouldSupersede(ProvenanceKubernetes, tie, ProvenanceDocker, tie)
	if aOverB == bOverA {
		t.Fatalf("expected exactly one direction to supersede on a full tie, got aOverB=%v bOverA=%v", aOverB, bOverA)
	}
}

func TestShouldSupersede_UnknownSource_NeverSupersedes(t *testing.T) {
	now := time.Now()
	if ShouldSupersede(ProvenanceObservedTelemetry, now, Provenance("NOT_A_REAL_SOURCE"), now) {
		t.Fatal("an unrecognized provenance must never be treated as authoritative")
	}
	if ShouldSupersede(Provenance("NOT_A_REAL_SOURCE"), now, ProvenanceObservedTelemetry, now) {
		t.Fatal("evidence should not supersede an already-invalid current record either -- fail closed, not open")
	}
}

func TestHigherPrecedence_ObservedTelemetryOutranksEverythingElse(t *testing.T) {
	for _, other := range []Provenance{ProvenanceDocker, ProvenanceKubernetes, ProvenanceDeclared, ProvenanceConfig, ProvenanceInferred} {
		if !HigherPrecedence(ProvenanceObservedTelemetry, other) {
			t.Errorf("expected OBSERVED_TELEMETRY to outrank %s", other)
		}
	}
}

func TestHigherPrecedence_DockerAndKubernetesOutrankDeclared(t *testing.T) {
	// Phase 7C's revised ordering: live platform observation (DOCKER/
	// KUBERNETES) outranks a declaration of intent (DECLARED), reversing
	// Phase 7B's original assumption -- see precedence.go's doc comment.
	if !HigherPrecedence(ProvenanceDocker, ProvenanceDeclared) {
		t.Error("expected DOCKER to outrank DECLARED under the revised ordering")
	}
	if !HigherPrecedence(ProvenanceKubernetes, ProvenanceDeclared) {
		t.Error("expected KUBERNETES to outrank DECLARED under the revised ordering")
	}
}

func TestHigherPrecedence_InferredIsLowest(t *testing.T) {
	for _, other := range []Provenance{ProvenanceObservedTelemetry, ProvenanceDocker, ProvenanceKubernetes, ProvenanceDeclared, ProvenanceConfig} {
		if HigherPrecedence(ProvenanceInferred, other) {
			t.Errorf("INFERRED must not outrank %s", other)
		}
	}
}
