// Package registry implements Phase 7B's canonical service registry: a
// persistent record of "what services are part of this Atlas-monitored
// system," distinct from internal/graph's DependencyGraph, which answers
// the narrower, ephemeral question "what dependencies are currently
// active." See docs/registry.md for the full architecture writeup.
package registry

import "time"

// Provenance records where a service record's existence was established.
// Only OBSERVED_TELEMETRY is implemented in Phase 7B. The other constants
// are a deliberate extension point for later phases -- declaring them now
// fixes the vocabulary future sources must use, without implementing any
// of them (there is no Docker/Kubernetes/config discovery in this phase).
type Provenance string

const (
	ProvenanceObservedTelemetry Provenance = "OBSERVED_TELEMETRY"
	ProvenanceDeclared          Provenance = "DECLARED"
	ProvenanceDocker            Provenance = "DOCKER"
	ProvenanceKubernetes        Provenance = "KUBERNETES"
	ProvenanceConfig            Provenance = "CONFIG"
	ProvenanceInferred          Provenance = "INFERRED"
)

// Status is the registry's own lifecycle state -- independent from
// DependencyGraph's retention-based existence. A service can be STALE or
// even RETIRED here while its graph node has already expired, and a
// service can be ACTIVE here while contributing zero edges to the graph
// right now (e.g. between requests). See EvaluateLifecycle in store.go for
// the exact, deterministic transition rules.
//
// The distinction this type exists to preserve, stated once so every
// caller keeps it straight: the REGISTRY answers "does Atlas know this
// service exists (or has it ever existed)?" -- ACTIVE/STALE/RETIRED are all
// answers of "yes," just with different recency. The telemetry GRAPH
// (internal/graph.DependencyGraph) answers a different question entirely,
// "has Atlas recently observed this service doing something?" -- and it
// answers that by the node simply being present or absent, with no
// lifecycle of its own. A RETIRED registry entry is not a diminished or
// half-deleted record; it is exactly as real as an ACTIVE one, just with
// stale evidence.
type Status string

const (
	StatusActive  Status = "ACTIVE"
	StatusStale   Status = "STALE"
	StatusRetired Status = "RETIRED"
)

// Confidence is the reliability class of the evidence currently backing a
// service's identity -- a fixed property of its Provenance (see
// ConfidenceFor), never a fabricated numeric score. There is no real basis
// for claiming e.g. "87% confidence" about a service's existence; a small,
// honest ordinal classification is all the evidence actually supports.
type Confidence string

const (
	// ConfidenceObserved means a real, live thing was actually seen: a
	// span/metric with this service.name (OBSERVED_TELEMETRY), or a live
	// container/pod object queried from an orchestrator API (DOCKER,
	// KUBERNETES -- not implemented yet, but would carry this same
	// confidence class when they are, since both are direct observation of
	// real running infrastructure, just via a different API than OTel).
	ConfidenceObserved Confidence = "OBSERVED"
	// ConfidenceDeclared means a human or a config file asserted the
	// service should exist (DECLARED, CONFIG) -- a statement of intent,
	// not proof that it is actually running right now.
	ConfidenceDeclared Confidence = "DECLARED"
	// ConfidenceInferred is the lowest class: a guess, not implemented by
	// anything in this codebase (INFERRED is vocabulary only).
	ConfidenceInferred Confidence = "INFERRED"
)

// ConfidenceFor returns the fixed reliability class for a Provenance
// source. This is a total function over every declared Provenance
// constant -- adding a new Provenance value without adding it here is a
// compile-time-safe but easy-to-miss mistake, so every case is listed
// explicitly rather than relying on a default.
func ConfidenceFor(p Provenance) Confidence {
	switch p {
	case ProvenanceObservedTelemetry, ProvenanceDocker, ProvenanceKubernetes:
		return ConfidenceObserved
	case ProvenanceDeclared, ProvenanceConfig:
		return ConfidenceDeclared
	case ProvenanceInferred:
		return ConfidenceInferred
	default:
		// An unrecognized provenance (should be unreachable given the
		// closed set of constants above) gets the lowest, most cautious
		// classification rather than a fabricated middle ground.
		return ConfidenceInferred
	}
}

// Evidence is one observation about a service's existence, from exactly
// one source at exactly one moment. Recording Evidence (Store.Record) is
// the only way a Service's canonical state changes -- there is no direct
// "set this service's status" write. Only OBSERVED_TELEMETRY producers
// exist today (internal/ingestion), but every field here is already
// source-agnostic, so a future DOCKER/KUBERNETES/DECLARED source needs
// only to construct one of these and call Record -- no registry rewrite.
type Evidence struct {
	ServiceName string
	Source      Provenance
	ObservedAt  time.Time
	// Metadata is reserved for source-specific facts a future source might
	// attach (e.g. DOCKER might record a container ID). Deliberately not
	// persisted yet in Phase 7C -- OBSERVED_TELEMETRY has nothing to put
	// here, and adding a column for a shape no real source has ever
	// populated would be exactly the kind of speculative field Atlas's own
	// discipline forbids. Adding persistence for it later, when a real
	// source needs it, is a small, additive schema change, not a rewrite.
	Metadata map[string]string
}

// Service is the canonical registry record for one logical service
// identity. Name is the stable identity key -- for OBSERVED_TELEMETRY, this
// is exactly the OTel `service.name` resource attribute, verbatim, never
// normalized/renamed/guessed. DisplayName exists as a schema extension
// point for a future provenance source that might supply a friendlier
// label than the raw telemetry identity (e.g. DECLARED config); for
// OBSERVED_TELEMETRY it is always identical to Name -- there is no other
// evidence to derive a display name from, so none is invented.
type Service struct {
	Name            string
	DisplayName     string
	Provenance      Provenance
	Status          Status
	FirstObservedAt time.Time
	LastObservedAt  time.Time
	// authorityObservedAt is the ObservedAt of whichever evidence most
	// recently WON precedence resolution and is therefore backing
	// Provenance/DisplayName right now -- distinct from LastObservedAt,
	// which advances on every sighting regardless of source. Unexported:
	// it is Store's own bookkeeping for ShouldSupersede's tie-break
	// (store.go), not something any consumer needs. See Store.Record.
	authorityObservedAt time.Time
	// LastTelemetryAt is nil until real telemetry has been observed. For
	// OBSERVED_TELEMETRY it is always set and always equal to
	// LastObservedAt; kept as a distinct field because a future DECLARED
	// record could exist with no telemetry at all.
	LastTelemetryAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
