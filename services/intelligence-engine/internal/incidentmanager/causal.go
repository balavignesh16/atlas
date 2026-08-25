package incidentmanager

import (
	"fmt"
	"log/slog"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/google/uuid"
)

// CausalAnalyzer re-attributes DEPENDENCY_ERROR evidence so rca.Engine.Analyze
// (unmodified) scores it for the service that actually failed, not the
// caller whose outgoing call to it failed. incidentdetector.evaluateDependencies
// files DEPENDENCY_FAILURE/DEPENDENCY_ERROR evidence under the CALLER
// (edge.SourceService) -- evidence that literally means "my dependency is
// failing" ends up scored as +20 supporting evidence for the caller, never
// for the callee that is actually broken, because a pure sink has no
// outgoing edge to earn that evidence type through in the first place. This
// is a re-attribution of already-real, already-observed evidence to its
// correct causal owner -- never fabrication of new evidence, never a change
// to any score weight, threshold, or ambiguity margin. It runs after
// incidentmanager.Correlator (so CorrelationGroupID/PrimaryIncidentID and
// the primary's merged EvidenceIDs already exist) and before rca.Engine.Analyze.
type CausalAnalyzer struct {
	// minObservations and dependencyErrorRateThreshold are NOT a new,
	// separately-invented definition of "failing dependency" -- they are
	// copied at construction time from the exact same
	// incidentdetector.Config values (MinObservations,
	// DependencyErrorRateThreshold) that incidentdetector.evaluateDependencies
	// already uses to decide whether an edge is failing. Reusing the same
	// Config instance's fields (see main.go wiring) guarantees this layer
	// can never disagree with M2.4 about what counts as a failing dependency.
	minObservations              int
	dependencyErrorRateThreshold float64
}

// NewCausalAnalyzer constructs a CausalAnalyzer. minObservations and
// dependencyErrorRateThreshold must be sourced from the same
// incidentdetector.Config used to construct the Detector, not re-declared.
func NewCausalAnalyzer(minObservations int, dependencyErrorRateThreshold float64) *CausalAnalyzer {
	return &CausalAnalyzer{
		minObservations:              minObservations,
		dependencyErrorRateThreshold: dependencyErrorRateThreshold,
	}
}

// isFailingEdge applies the identical two-condition test
// incidentdetector.evaluateDependencies uses: a minimum observation-count
// gate, then a strict error-rate threshold.
func (c *CausalAnalyzer) isFailingEdge(edge *correlationmodel.DependencyEdge) bool {
	if edge.CallCount < int64(c.minObservations) {
		return false
	}
	errRate := float64(edge.ErrorCount) / float64(edge.CallCount)
	return errRate > c.dependencyErrorRateThreshold
}

// ResolveCausalSinksFor returns the set of terminal "sink" services reachable
// from caller by following FAILING edges within groupServices. If caller has
// no currently-failing outgoing edge within the group, caller is itself the
// (only) sink. Branches (a caller with multiple failing outgoing edges)
// resolve to multiple sinks. Cycle-safe: returns an empty set if every
// reachable branch loops back without reaching a clean sink -- never
// fabricates a redirection target that doesn't structurally exist.
func (c *CausalAnalyzer) ResolveCausalSinksFor(caller string, groupServices map[string]bool, edges []*correlationmodel.DependencyEdge) map[string]bool {
	return c.resolveCausalSinks(caller, groupServices, edges, make(map[string]bool))
}

func (c *CausalAnalyzer) resolveCausalSinks(caller string, groupServices map[string]bool, edges []*correlationmodel.DependencyEdge, visited map[string]bool) map[string]bool {
	if visited[caller] {
		return map[string]bool{}
	}
	visited[caller] = true

	var failingTargets []string
	for _, e := range edges {
		if e.SourceService != caller || e.TargetService == caller {
			continue
		}
		if !groupServices[e.TargetService] {
			// Target isn't part of this correlated incident group -- nothing
			// in the current picture to redirect to. Leave attribution as-is
			// rather than inventing a relationship the group doesn't show.
			continue
		}
		if c.isFailingEdge(e) {
			failingTargets = append(failingTargets, e.TargetService)
		}
	}

	if len(failingTargets) == 0 {
		return map[string]bool{caller: true}
	}

	sinks := make(map[string]bool)
	for _, target := range failingTargets {
		for s := range c.resolveCausalSinks(target, groupServices, edges, visited) {
			sinks[s] = true
		}
	}
	return sinks
}

// ApplyCausalAttribution re-attributes DEPENDENCY_ERROR evidence within each
// correlated group's primary incident. Metadata-only: it edits which
// existing evidence IDs the primary's EvidenceIDs list references, and adds
// new evidence.Evidence records that redirect already-real observed data --
// it never changes Incident.Status, never touches incidentmanager.Correlator's
// grouping/primary-selection output, and never calls into rca.Engine.
//
// Fail-open: any panic is recovered and logged; on error, incidents are left
// exactly as Correlator produced them, matching this package's existing
// fail-open convention (see Correlator.Correlate).
func (c *CausalAnalyzer) ApplyCausalAttribution(incidents []*incidentmodel.Incident, depGraph *graph.DependencyGraph, evStore *evidence.Store) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("CausalAnalyzer: recovered from panic, failing open (evidence left unredirected)", "error", r)
		}
	}()

	if len(incidents) == 0 || depGraph == nil || evStore == nil {
		return
	}

	edges := depGraph.GetEdges()

	byGroup := make(map[string][]*incidentmodel.Incident)
	for _, inc := range incidents {
		if inc.CorrelationGroupID == "" {
			continue
		}
		byGroup[inc.CorrelationGroupID] = append(byGroup[inc.CorrelationGroupID], inc)
	}

	for _, members := range byGroup {
		var primary *incidentmodel.Incident
		for _, m := range members {
			if m.PrimaryIncidentID == m.IncidentID {
				primary = m
				break
			}
		}
		if primary == nil {
			continue // defensive; Correlator always sets this
		}
		c.applyToPrimary(primary, members, edges, evStore)
	}
}

func (c *CausalAnalyzer) applyToPrimary(primary *incidentmodel.Incident, members []*incidentmodel.Incident, edges []*correlationmodel.DependencyEdge, evStore *evidence.Store) {
	groupServices := make(map[string]bool, len(members))
	for _, m := range members {
		groupServices[m.RootService] = true
	}

	evs := evStore.GetAll(primary.EvidenceIDs)

	suppress := make(map[string]bool)        // evidenceID -> remove from primary.EvidenceIDs
	redirectTargets := make(map[string]bool) // resolved sink service -> needs one new evidence entry
	redirectedAny := false

	for _, ev := range evs {
		if ev.Type != evidence.EvidenceTypeDependencyError {
			continue
		}
		if !groupServices[ev.Service] {
			continue // not a member of this correlated group; leave untouched
		}

		sinks := c.ResolveCausalSinksFor(ev.Service, groupServices, edges)

		onlySelf := len(sinks) == 1 && sinks[ev.Service]
		if len(sinks) == 0 || onlySelf {
			// No clean resolution target (cycle, or the edge that generated
			// this evidence is no longer observably failing) -- leave the
			// evidence exactly as originally attributed.
			continue
		}

		suppress[ev.EvidenceID] = true
		redirectedAny = true
		for sink := range sinks {
			redirectTargets[sink] = true
		}
	}

	if !redirectedAny {
		return
	}

	newIDs := make([]string, 0, len(primary.EvidenceIDs)+len(redirectTargets))
	for _, id := range primary.EvidenceIDs {
		if !suppress[id] {
			newIDs = append(newIDs, id)
		}
	}
	// Exactly one new evidence entry per resolved sink, regardless of how
	// many suppressed callers redirected to it -- redirectTargets is a set,
	// so this cannot double-count a destination service.
	for sink := range redirectTargets {
		ev := evidence.Evidence{
			EvidenceID:  uuid.New().String(),
			Type:        evidence.EvidenceTypeDependencyError,
			Timestamp:   primary.LastUpdatedAt,
			Service:     sink,
			Description: fmt.Sprintf("%s is the resolved causal origin of a failing dependency chain observed within this incident", sink),
			Source:      "CausalAnalyzer",
		}
		evStore.Add(ev)
		newIDs = append(newIDs, ev.EvidenceID)
	}
	primary.EvidenceIDs = newIDs
}
