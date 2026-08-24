package propagation

import (
	"time"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

type Analyzer struct {
	graphEngine *graph.DependencyGraph
	corrEngine  *correlation.Engine
}

func NewAnalyzer(graphEngine *graph.DependencyGraph, corrEngine *correlation.Engine) *Analyzer {
	return &Analyzer{
		graphEngine: graphEngine,
		corrEngine:  corrEngine,
	}
}

// CheckTemporalPrecedence returns true if failure in candidate started BEFORE failure in target.
//
// M2.7.2 investigated switching this comparison from StartTime to EndTime
// (the deepest failing span in a nested chain completes first, since it
// fails immediately with nothing further to wait on) to let this dormant
// mechanism actually activate. That change worked as designed -- verified
// with real trace data -- but was reverted after live testing surfaced a
// worse emergent effect: a middle-tier caller (e.g. order-service) already
// carries its own DEPENDENCY_FAILURE evidence from rca.Engine's existing,
// unmodified scoring (its own error rate + a failing outgoing dependency),
// and stacking a newly-active precedence bonus on top of that let it
// confidently (HIGH, score 80) outrank the true root cause (payment, which
// as a pure sink can never earn the dependency-failure bonus) -- turning
// M2.7.1's honest AMBIGUOUS(order, gateway) into a confident WRONG answer.
// That's a direct violation of "do not force a root cause" / "preserve
// AMBIGUOUS when evidence is insufficient", and rca.Engine's scoring
// (where the actual fix would need to live -- weighing precedence against
// how many distinct evidence types a candidate already holds) is out of
// bounds for this milestone. TraceID population and status classification
// stay fixed regardless (real, safe, independently useful); this specific
// mechanism stays dormant, StartTime-based, exactly as before, until a
// future milestone can touch rca.Engine's scoring formula itself.
func (a *Analyzer) CheckTemporalPrecedence(candidate string, target string, inc *incidentmodel.Incident) (bool, time.Time, time.Time) {
	// Find earliest error timestamp for candidate vs target in the incident's traces
	var candEarliest, targetEarliest time.Time
	candFound, targetFound := false, false

	for _, traceID := range inc.TraceIDs {
		traceDTO, ok := a.corrEngine.GetTrace(traceID)
		if !ok {
			continue
		}

		for _, span := range traceDTO.Spans {
			if span.Status == "ERROR" || span.Status == "5xx" {
				if span.ServiceName == candidate {
					if !candFound || span.StartTime.Before(candEarliest) {
						candEarliest = span.StartTime
						candFound = true
					}
				}
				if span.ServiceName == target {
					if !targetFound || span.StartTime.Before(targetEarliest) {
						targetEarliest = span.StartTime
						targetFound = true
					}
				}
			}
		}
	}

	if candFound && targetFound && candEarliest.Before(targetEarliest) {
		return true, candEarliest, targetEarliest
	}
	return false, candEarliest, targetEarliest
}

// IsUpstream checks if candidate is upstream of target (candidate calls target eventually).
// Wait! If Gateway -> Order -> Payment. 
// Gateway is upstream of Order. Payment is downstream of Order.
// "If A is upstream of B but A fails AFTER B, A should NOT receive temporal-causality points."
// "If A fails before B, and A -> B, and B failures increase afterward, A receives temporal propagation evidence."
// Let's use graph BFS/DFS to check path.
func (a *Analyzer) IsPath(source string, target string) bool {
	// Simple BFS
	edges := a.graphEngine.GetEdges()
	visited := make(map[string]bool)
	queue := []string{source}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr == target {
			return true
		}
		if visited[curr] {
			continue
		}
		visited[curr] = true

		for _, edge := range edges {
			if edge.SourceService == curr {
				queue = append(queue, edge.TargetService)
			}
		}
	}
	return false
}

// CheckDownstreamPropagation checks if candidate's failure propagated to upstream callers.
// Wait, "Downstream propagation": Gateway -> Order -> Payment. Payment failure propagates to Order.
// So Payment is downstream of Order. Payment failure propagates UP to Order!
// The term "downstream propagation" usually means failure propagates to the services that depend on it (the upstream callers).
func (a *Analyzer) CheckPropagation(candidate string, affected []string, inc *incidentmodel.Incident) []evidence.Evidence {
	var evs []evidence.Evidence
	// Check temporal precedence against all other affected services
	for _, other := range affected {
		if other == candidate {
			continue
		}
		
		// If candidate is called by other (other -> candidate)
		if a.IsPath(other, candidate) {
			prec, cTime, oTime := a.CheckTemporalPrecedence(candidate, other, inc)
			if prec {
				ev := evidence.Evidence{
					EvidenceID:  "EV-TEMP-" + candidate + "-" + other,
					Type:        evidence.EvidenceTypeTemporalSequence,
					Timestamp:   time.Now(),
					Service:     candidate,
					Operation:   "N/A",
					Description: candidate + " degradation preceded " + other + " failures by " + oTime.Sub(cTime).String(),
					Source:      "PropagationAnalyzer",
				}
				evs = append(evs, ev)
			}
		}
	}
	return evs
}
