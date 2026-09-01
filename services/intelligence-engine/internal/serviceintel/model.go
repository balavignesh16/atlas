// Package serviceintel composes a read-only, per-service view from three
// existing sources -- internal/registry (persistent identity/evidence),
// internal/graph (live dependency topology), and internal/incidentmanager
// (incident history, with RCA output already materialized onto each
// Incident) -- into one structured object a future AI reasoning layer
// could consume, without itself doing any AI, discovery, persistence, or
// mutation. See docs/registry.md's "Service Intelligence" section for the
// full architecture writeup.
//
// This package deliberately imports none of the six frozen packages
// (internal/execution, internal/rca, internal/incidentdetector,
// internal/remediation, internal/blast, internal/propagation) -- RCA's
// output is already sitting on incidentmodel.Incident by the time anything
// reads it, so there is nothing to import from internal/rca itself.
package serviceintel

import "time"

// RegistryView reflects internal/registry's knowledge of a service. Known
// is false when the registry has never recorded this name -- in that case
// every other field is the zero value and MUST NOT be treated as real data
// by a caller; the JSON encoding omits them entirely (see
// internal/httpapi/intelligence.go) rather than emitting fabricated zeros.
type RegistryView struct {
	Known           bool
	Status          string
	Provenance      string
	Confidence      string
	FirstObservedAt time.Time
	LastObservedAt  time.Time
}

// DependencyView is a direct field-for-field transcription of one
// correlationmodel.DependencyEdge, from the perspective of the service
// being queried -- Service names whichever endpoint is NOT that service
// (the caller, for an incoming edge; the callee, for an outgoing one).
// CallCount/ErrorCount/AverageDurationMs/FirstObserved/LastObserved are
// copied verbatim, never recomputed.
type DependencyView struct {
	Service           string
	CallCount         int64
	ErrorCount        int64
	AverageDurationMs int64
	FirstObserved     time.Time
	LastObserved      time.Time
}

// DependenciesView holds both directions. Both slices are always non-nil
// (possibly empty) so JSON encoding always produces `[]`, never `null` --
// "no dependencies observed" is real, meaningful information, not an
// absent value.
type DependenciesView struct {
	Incoming []DependencyView
	Outgoing []DependencyView
}

// IncidentSummary is a minimal, real projection of incidentmodel.Incident
// -- exactly the fields Phase 7D's design calls for, not the full incident
// record (callers needing more already have GET /api/v1/incidents/{id}).
// Confidence is Incident.Confidence verbatim (RCA's own output, already
// materialized by the time this reads it) and may be empty if RCA has not
// run for this incident -- never invented.
type IncidentSummary struct {
	IncidentID  string
	Status      string
	Severity    string
	Title       string
	StartedAt   time.Time
	RootService string
	Confidence  string
}

// ServiceIntelligence is the composed, deterministic result. GeneratedAt is
// always the `now` passed to Assembler.Build, never wall-clock time read
// internally -- this keeps Build (and therefore this whole type) fully
// deterministic for a fixed input.
type ServiceIntelligence struct {
	ServiceName       string
	Registry          RegistryView
	Dependencies      DependenciesView
	RelevantIncidents []IncidentSummary
	GeneratedAt       time.Time
}
