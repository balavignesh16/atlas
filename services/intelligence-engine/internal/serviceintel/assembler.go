package serviceintel

import (
	"fmt"
	"sort"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/registry"
)

// relevantIncidentsLimit bounds how many incidents Build returns, most
// recent first. A local, unexported constant -- not imported from any
// other package (frozen or otherwise) merely to reuse a number; Phase 4's
// executions page uses a conceptually similar bound for the same reason
// (an unbounded per-service incident list would grow without limit over a
// long-running session), but that constant lives in frontend code and
// coupling this package to it would serve no purpose.
const relevantIncidentsLimit = 10

// Assembler composes ServiceIntelligence from three independent, already-
// existing sources. It holds no state of its own beyond references to
// those sources, performs no writes, and caches nothing -- every Build
// call re-reads all three sources fresh.
type Assembler struct {
	registry  *registry.Store
	graph     *graph.DependencyGraph
	incidents *incidentmanager.Manager
}

func NewAssembler(store *registry.Store, depGraph *graph.DependencyGraph, incManager *incidentmanager.Manager) *Assembler {
	return &Assembler{registry: store, graph: depGraph, incidents: incManager}
}

// Build composes a ServiceIntelligence for name as of now. ok is false
// only when name is unknown to ALL THREE sources (registry, graph, and
// incident history) -- a service known to even one of them is a real,
// constructible result, never a 404. now is used verbatim for
// GeneratedAt; Build never reads the wall clock itself, which is what
// keeps it deterministic for a fixed set of inputs.
func (a *Assembler) Build(name string, now time.Time) (ServiceIntelligence, bool, error) {
	result := ServiceIntelligence{
		ServiceName: name,
		Dependencies: DependenciesView{
			Incoming: make([]DependencyView, 0),
			Outgoing: make([]DependencyView, 0),
		},
		RelevantIncidents: make([]IncidentSummary, 0),
		GeneratedAt:       now,
	}

	svc, known, err := a.registry.Get(name)
	if err != nil {
		return ServiceIntelligence{}, false, fmt.Errorf("serviceintel: registry lookup for %s: %w", name, err)
	}
	result.Registry.Known = known
	if known {
		result.Registry.Status = string(svc.Status)
		result.Registry.Provenance = string(svc.Provenance)
		result.Registry.Confidence = string(registry.ConfidenceFor(svc.Provenance))
		result.Registry.FirstObservedAt = svc.FirstObservedAt
		result.Registry.LastObservedAt = svc.LastObservedAt
	}

	incoming, outgoing := a.graph.GetServiceDependencies(name)
	for _, edge := range incoming {
		result.Dependencies.Incoming = append(result.Dependencies.Incoming, toDependencyView(edge, true))
	}
	for _, edge := range outgoing {
		result.Dependencies.Outgoing = append(result.Dependencies.Outgoing, toDependencyView(edge, false))
	}
	// GetServiceDependencies iterates an internal map (see internal/graph),
	// so its return order is not guaranteed across calls. Sorting here --
	// not in internal/graph, which stays unmodified -- is what makes this
	// package's own output deterministic regardless.
	sort.Slice(result.Dependencies.Incoming, func(i, j int) bool {
		return result.Dependencies.Incoming[i].Service < result.Dependencies.Incoming[j].Service
	})
	sort.Slice(result.Dependencies.Outgoing, func(i, j int) bool {
		return result.Dependencies.Outgoing[i].Service < result.Dependencies.Outgoing[j].Service
	})

	var relevant []*incidentmodel.Incident
	for _, inc := range a.incidents.GetAllIncidents() {
		if isRelevantIncident(inc, name) {
			relevant = append(relevant, inc)
		}
	}
	sortIncidentsDeterministically(relevant)
	if len(relevant) > relevantIncidentsLimit {
		relevant = relevant[:relevantIncidentsLimit]
	}
	for _, inc := range relevant {
		result.RelevantIncidents = append(result.RelevantIncidents, toIncidentSummary(inc))
	}

	ok := known || len(incoming) > 0 || len(outgoing) > 0 || len(relevant) > 0
	if !ok {
		return ServiceIntelligence{}, false, nil
	}

	return result, true, nil
}

// toDependencyView maps one edge into the view of the service being
// queried. incoming=true means `name` is the edge's target (the edge's
// SOURCE is the "other" service calling in); incoming=false means `name`
// is the edge's source (the edge's TARGET is the "other" service being
// called). This selection of which endpoint is "the other side" is the
// only interpretation applied -- every numeric/timestamp field is copied
// verbatim from the real edge.
func toDependencyView(edge *correlationmodel.DependencyEdge, incoming bool) DependencyView {
	other := edge.TargetService
	if incoming {
		other = edge.SourceService
	}
	return DependencyView{
		Service:           other,
		CallCount:         edge.CallCount,
		ErrorCount:        edge.ErrorCount,
		AverageDurationMs: edge.AverageDurationMs,
		FirstObserved:     edge.FirstObserved,
		LastObserved:      edge.LastObserved,
	}
}

// isRelevantIncident matches Phase 7D's documented relevance rule exactly:
// the service is either the incident's RootService or appears in its
// AffectedServices. No fuzzy matching, no name normalization.
func isRelevantIncident(inc *incidentmodel.Incident, service string) bool {
	if inc.RootService == service {
		return true
	}
	for _, affected := range inc.AffectedServices {
		if affected == service {
			return true
		}
	}
	return false
}

// sortIncidentsDeterministically orders most-recent-first by StartedAt,
// breaking ties (including a full tie on identical timestamps) by
// IncidentID so the result never depends on GetAllIncidents' own
// (unspecified) return order.
func sortIncidentsDeterministically(incidents []*incidentmodel.Incident) {
	sort.Slice(incidents, func(i, j int) bool {
		if !incidents[i].StartedAt.Equal(incidents[j].StartedAt) {
			return incidents[i].StartedAt.After(incidents[j].StartedAt)
		}
		return incidents[i].IncidentID < incidents[j].IncidentID
	})
}

func toIncidentSummary(inc *incidentmodel.Incident) IncidentSummary {
	return IncidentSummary{
		IncidentID:  inc.IncidentID,
		Status:      string(inc.Status),
		Severity:    string(inc.Severity),
		Title:       inc.Title,
		StartedAt:   inc.StartedAt,
		RootService: inc.RootService,
		Confidence:  inc.Confidence,
	}
}
