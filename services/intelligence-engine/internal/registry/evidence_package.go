package registry

import "time"

// EvidencePackage is the structured, deterministic bundle a future AI
// reasoning layer would consume, so it reasons over one well-defined
// contract instead of querying registry/graph/incident internals
// directly. Nothing calls this from a live HTTP endpoint yet -- there is
// no AI consumer in this codebase (internal/aireasoning's Gemini provider
// and internal/remediation's AI planner are both unimplemented
// placeholders; FakeProvider/FakePlanner are what actually run). This
// exists purely as an architectural preparation: a real, tested contract
// ready for whenever a real AI layer exists to call it.
//
// Every field is either a real value the caller supplied or explicitly the
// zero value / nil slice -- BuildEvidencePackage never invents a value for
// information it wasn't given. In particular:
//   - Dependencies is real, caller-supplied topology (e.g. from
//     internal/graph.DependencyGraph.GetServiceDependencies), never derived
//     by this package itself -- internal/registry does not import
//     internal/graph, keeping this contract decoupled from any one
//     evidence producer.
//   - IncidentFacts is real, caller-supplied incident summary strings
//     (e.g. from internal/incidentmanager). Nil means "the caller didn't
//     supply any," never "confirmed that none exist."
//
// See docs/registry.md's "AI-Ready Evidence Contract" section for how a
// future AI layer should map these fields to FACT/INFERENCE/UNKNOWN: every
// field here is FACT (either directly observed or deterministically
// derived); only a future AI layer's own output would ever be INFERENCE,
// and it must never overwrite these fields with an assumption.
type EvidencePackage struct {
	ServiceName     string
	LifecycleStatus Status
	Provenance      Provenance
	Confidence      Confidence
	FirstObservedAt time.Time
	LastObservedAt  time.Time
	Dependencies    []string
	IncidentFacts   []string
	GeneratedAt     time.Time
}

// BuildEvidencePackage assembles a package for one known service from the
// registry's own record plus whatever dependency/incident facts the caller
// supplies. ok is false if the service is not in the registry at all --
// there is nothing to build a package around.
func (s *Store) BuildEvidencePackage(name string, dependencies []string, incidentFacts []string, now time.Time) (EvidencePackage, bool, error) {
	svc, ok, err := s.Get(name)
	if err != nil {
		return EvidencePackage{}, false, err
	}
	if !ok {
		return EvidencePackage{}, false, nil
	}

	return EvidencePackage{
		ServiceName:     svc.Name,
		LifecycleStatus: svc.Status,
		Provenance:      svc.Provenance,
		Confidence:      ConfidenceFor(svc.Provenance),
		FirstObservedAt: svc.FirstObservedAt,
		LastObservedAt:  svc.LastObservedAt,
		Dependencies:    dependencies,
		IncidentFacts:   incidentFacts,
		GeneratedAt:     now,
	}, true, nil
}
