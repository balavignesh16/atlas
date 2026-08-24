package incidentmanager

import (
	"log/slog"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/google/uuid"
)

// Correlator links currently-open incidents that are connected through the
// observed M2.3 dependency graph and occurred within a bounded time window,
// so RCA can see a cascade's full picture instead of scoring each service in
// isolation. It is metadata-only: it never changes Incident.Status, never
// merges/hides incidents, and never touches rca.Engine. Correlation must run
// before RCA is evaluated for the affected incidents.
type Correlator struct {
	windowSeconds int
}

// NewCorrelator creates a Correlator. windowSeconds bounds how far apart two
// graph-connected incidents' StartedAt times may be and still be considered
// part of the same cascade; a non-positive value falls back to 20s.
func NewCorrelator(windowSeconds int) *Correlator {
	if windowSeconds <= 0 {
		windowSeconds = 20
	}
	return &Correlator{windowSeconds: windowSeconds}
}

// Correlate groups incidents, selects a caller/callee-aware primary for each
// group, and enriches the primary's AffectedServices and EvidenceIDs with
// the union of the group's data so the unmodified rca.Engine can evaluate
// the full cascade. It mutates the given incidents in place.
//
// Fail-open: if incidents or depGraph is empty/nil, or anything unexpected
// happens, every incident is left as its own single-member group rather than
// blocking detection/RCA from proceeding.
func (c *Correlator) Correlate(incidents []*incidentmodel.Incident, depGraph *graph.DependencyGraph, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("Correlator: recovered from panic, failing open (incidents left uncorrelated)", "error", r)
		}
	}()

	if len(incidents) == 0 || depGraph == nil {
		return
	}

	edges := depGraph.GetEdges()

	groups := c.group(incidents, edges)

	for _, members := range groups {
		c.applyGroup(incidents, members, edges)
	}
}

// group computes connected components over the incidents: two incidents are
// linked only if their root services are connected by an observed dependency
// edge (in either direction) AND their StartedAt times fall within the
// configured window of each other. Same-root-service pairs are skipped here;
// that case is already handled by M2.4's own fingerprint-based dedup.
func (c *Correlator) group(incidents []*incidentmodel.Incident, edges []*correlationmodel.DependencyEdge) [][]int {
	n := len(incidents)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	connected := func(serviceA, serviceB string) bool {
		for _, e := range edges {
			if (e.SourceService == serviceA && e.TargetService == serviceB) ||
				(e.SourceService == serviceB && e.TargetService == serviceA) {
				return true
			}
		}
		return false
	}

	windowDur := time.Duration(c.windowSeconds) * time.Second
	withinWindow := func(a, b time.Time) bool {
		d := a.Sub(b)
		if d < 0 {
			d = -d
		}
		return d <= windowDur
	}

	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if incidents[i].RootService == "" || incidents[j].RootService == "" {
				continue
			}
			if incidents[i].RootService == incidents[j].RootService {
				continue
			}
			if !withinWindow(incidents[i].StartedAt, incidents[j].StartedAt) {
				continue
			}
			if !connected(incidents[i].RootService, incidents[j].RootService) {
				continue
			}
			union(i, j)
		}
	}

	byRoot := make(map[int][]int)
	for i := 0; i < n; i++ {
		root := find(i)
		byRoot[root] = append(byRoot[root], i)
	}

	groups := make([][]int, 0, len(byRoot))
	for _, members := range byRoot {
		groups = append(groups, members)
	}
	return groups
}

// applyGroup selects the caller/callee-aware primary for one group and
// stamps correlation metadata on every member. A member is eligible as
// primary only if no other member's service is something it calls (it must
// not depend, directly, on another failing member in the group) -- i.e. it
// is a "sink" within the group's induced call subgraph. Every incident,
// including singleton groups, receives metadata so the fields are always
// meaningful: a standalone incident gets its own IncidentID as
// PrimaryIncidentID and an empty RelatedIncidentIDs.
func (c *Correlator) applyGroup(incidents []*incidentmodel.Incident, members []int, edges []*correlationmodel.DependencyEdge) {
	groupID := uuid.New().String()

	memberServices := make(map[string]bool, len(members))
	for _, idx := range members {
		memberServices[incidents[idx].RootService] = true
	}

	// callsAnotherMember reports whether `service` has an observed outgoing
	// edge to another member's service -- i.e. it depends on (calls) a
	// service that is also part of this failing group.
	callsAnotherMember := func(service string) bool {
		for _, e := range edges {
			if e.SourceService == service && e.TargetService != service && memberServices[e.TargetService] {
				return true
			}
		}
		return false
	}

	var sinkCandidates []int
	for _, idx := range members {
		if !callsAnotherMember(incidents[idx].RootService) {
			sinkCandidates = append(sinkCandidates, idx)
		}
	}

	pool := sinkCandidates
	fellBack := false
	if len(pool) == 0 {
		// No pure sink (e.g. an observed call cycle). Degraded fallback:
		// consider every member rather than silently dropping the group --
		// a caller is never preferred over a callee when a sink DOES exist,
		// this path only triggers when the graph itself has no clean sink.
		pool = members
		fellBack = true
	}

	primaryIdx := pool[0]
	for _, idx := range pool[1:] {
		if incidents[idx].StartedAt.Before(incidents[primaryIdx].StartedAt) {
			primaryIdx = idx
		}
	}
	primary := incidents[primaryIdx]

	if fellBack {
		slog.Warn("Correlator: no caller/callee sink found in correlation group (possible observed call cycle); falling back to earliest-started incident as primary",
			"correlationGroupId", groupID, "primaryIncidentId", primary.IncidentID)
	}

	allIDs := make([]string, 0, len(members))
	for _, idx := range members {
		allIDs = append(allIDs, incidents[idx].IncidentID)
	}

	unionServices := make(map[string]bool, len(members))
	unionEvidence := make(map[string]bool)
	for _, idx := range members {
		inc := incidents[idx]
		unionServices[inc.RootService] = true
		for _, eid := range inc.EvidenceIDs {
			unionEvidence[eid] = true
		}
	}

	for _, idx := range members {
		inc := incidents[idx]
		inc.CorrelationGroupID = groupID
		inc.PrimaryIncidentID = primary.IncidentID

		related := make([]string, 0, len(allIDs)-1)
		for _, id := range allIDs {
			if id != inc.IncidentID {
				related = append(related, id)
			}
		}
		inc.RelatedIncidentIDs = related
	}

	// Only the primary's AffectedServices/EvidenceIDs are enriched with the
	// group's union. rca.Engine.Analyze reads both fields unmodified: it
	// scores every service in AffectedServices using only the evidence
	// listed in EvidenceIDs, so both must carry the full cascade for RCA to
	// see it. Non-primary incidents are left exactly as M2.4 produced them.
	mergedServices := make([]string, 0, len(unionServices))
	for s := range unionServices {
		mergedServices = append(mergedServices, s)
	}
	primary.AffectedServices = mergedServices

	mergedEvidence := make([]string, 0, len(unionEvidence))
	for e := range unionEvidence {
		mergedEvidence = append(mergedEvidence, e)
	}
	primary.EvidenceIDs = mergedEvidence
}
