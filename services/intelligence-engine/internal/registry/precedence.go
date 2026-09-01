package registry

import "time"

// precedence ranks Provenance sources from most to least authoritative,
// lowest number wins. This governs what happens when a future phase adds a
// second provenance source and it disagrees with an existing record (e.g.
// a DECLARED config entry naming a service OBSERVED_TELEMETRY has never
// seen, or vice versa). No such conflict is possible yet --
// OBSERVED_TELEMETRY is the only implemented source, so this table
// currently governs nothing in production; it exists so the resolution
// mechanism (ShouldSupersede, below) is real, tested, and ready before it's
// needed, not designed from scratch under pressure when a second source
// finally arrives.
//
// Phase 7C revises the ordering Phase 7B originally shipped. That version
// ranked DECLARED above DOCKER/KUBERNETES ("declared intent outranks
// platform inference"). On review that gets it backwards: DECLARED means a
// human or a config file asserted a service *should* exist -- a statement
// of intent that can be stale, aspirational, or simply wrong (a
// docker-compose entry for a service nobody actually starts). DOCKER and
// KUBERNETES, once implemented, would mean Atlas queried the orchestrator's
// own live API and found a real, currently-running container or pod --
// that is direct observation of present infrastructure, not a declaration
// about it. Direct observation of real, current state outranks a
// declaration of intent, which outranks a value merely referenced from
// configuration (CONFIG), which outranks a plain guess (INFERRED).
var precedence = map[Provenance]int{
	ProvenanceObservedTelemetry: 0, // real evidence: a span/metric was actually seen
	ProvenanceDocker:            1, // real evidence: a live container was actually queried
	ProvenanceKubernetes:        1, // real evidence: a live pod/deployment was actually queried
	ProvenanceDeclared:          2, // a human or config asserted it should exist
	ProvenanceConfig:            3, // inferred from a referenced env var/URL, not asserted directly
	ProvenanceInferred:          4, // lowest: a guess, e.g. a future AI-assisted inference
}

// HigherPrecedence reports whether a has strictly higher precedence than b.
func HigherPrecedence(a, b Provenance) bool {
	rankA, okA := precedence[a]
	rankB, okB := precedence[b]
	if !okA || !okB {
		return false
	}
	return rankA < rankB
}

// ShouldSupersede reports whether newEvidence should become authoritative
// over a service's currently-recorded (source, observedAt) pair for
// IDENTITY fields (Provenance, DisplayName) -- never for LastObservedAt,
// which always advances to the latest evidence regardless of source (see
// Store.Record): "is this service still around" is a different question
// from "which source do we trust for its identity," and a weak source
// re-confirming a service's continued existence should not be discarded
// just because a stronger source described it once, long ago.
//
// Deterministic and safe to fold over any set of Evidence in any order:
//   - Higher precedence (lower rank number) always wins, regardless of
//     timestamp -- a still-fresh OBSERVED_TELEMETRY record is not displaced
//     by a newer but weaker DECLARED entry.
//   - Equal precedence is broken by the later ObservedAt (the normal case:
//     repeated telemetry from the same source, latest wins).
//   - A full tie (equal precedence AND equal timestamp, only possible in
//     tests/synthetic data) is broken by comparing the source name, purely
//     so the function never returns a different answer for the same two
//     inputs depending on which happened to be recorded first.
func ShouldSupersede(currentSource Provenance, currentAt time.Time, newSource Provenance, newAt time.Time) bool {
	currentRank, okCurrent := precedence[currentSource]
	newRank, okNew := precedence[newSource]
	if !okCurrent || !okNew {
		return false
	}
	if newRank != currentRank {
		return newRank < currentRank
	}
	if !newAt.Equal(currentAt) {
		return newAt.After(currentAt)
	}
	return newSource > currentSource
}
