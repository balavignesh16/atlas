package rca

import (
	"fmt"
	"sort"
	"strings"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/propagation"
	"github.com/google/uuid"
)

type Engine struct {
	evStore     *evidence.Store
	propAnalyzer *propagation.Analyzer
	graphEngine *graph.DependencyGraph
}

func NewEngine(evStore *evidence.Store, propAnalyzer *propagation.Analyzer, graphEngine *graph.DependencyGraph) *Engine {
	return &Engine{
		evStore:      evStore,
		propAnalyzer: propAnalyzer,
		graphEngine:  graphEngine,
	}
}

func (e *Engine) getConfidence(score int) string {
	if score < 40 {
		return "LOW"
	}
	if score < 70 {
		return "MEDIUM"
	}
	return "HIGH"
}

func (e *Engine) Analyze(inc *incidentmodel.Incident) {
	if inc == nil || len(inc.AffectedServices) == 0 {
		return
	}

	candidates := make([]*RCACandidate, 0)

	// Build candidates from affected services
	for _, srv := range inc.AffectedServices {
		cand := &RCACandidate{
			Service:     srv,
			Operation:   "N/A", // Can be refined if we want to isolate operation
			Score:       0,
			EvidenceIDs: make([]string, 0),
			Reasoning:   make([]string, 0),
		}

		// Check basic evidences
		hasErrorIncrease := false
		hasLatencyIncrease := false
		hasDepFailure := false

		// We can look at the evidence already attached to the incident
		evs := e.evStore.GetAll(inc.EvidenceIDs)
		for _, ev := range evs {
			if ev.Service == srv {
				if ev.Type == evidence.EvidenceTypeErrorRate || ev.Type == evidence.EvidenceTypeSpanError {
					if !hasErrorIncrease {
						cand.Score += 25
						hasErrorIncrease = true
						cand.Reasoning = append(cand.Reasoning, "Error rate increased.")
						cand.EvidenceIDs = append(cand.EvidenceIDs, ev.EvidenceID)
					}
				}
				if ev.Type == evidence.EvidenceTypeLatency {
					if !hasLatencyIncrease {
						cand.Score += 20
						hasLatencyIncrease = true
						cand.Reasoning = append(cand.Reasoning, "Latency exceeded threshold.")
						cand.EvidenceIDs = append(cand.EvidenceIDs, ev.EvidenceID)
					}
				}
				if ev.Type == evidence.EvidenceTypeDependencyError {
					if !hasDepFailure {
						cand.Score += 20
						hasDepFailure = true
						cand.Reasoning = append(cand.Reasoning, "Dependency failure rate exceeded threshold.")
						cand.EvidenceIDs = append(cand.EvidenceIDs, ev.EvidenceID)
					}
				}
			}
		}

		// Propagation and Precedence
		propEvs := e.propAnalyzer.CheckPropagation(srv, inc.AffectedServices, inc)
		if len(propEvs) > 0 {
			cand.Score += 20 // Temporal precedence
			cand.Score += 10 // Downstream propagation
			
			for _, pev := range propEvs {
				e.evStore.Add(pev)
				cand.EvidenceIDs = append(cand.EvidenceIDs, pev.EvidenceID)
			}
			cand.Reasoning = append(cand.Reasoning, "Degradation preceded upstream service failures (temporal precedence & downstream propagation).")
		}

		// Healthy independent dependency evidence
		// e.g. Payment depends on external API, or Order depends on Inventory and Inventory is healthy
		edges := e.graphEngine.GetEdges()
		healthyUpstreamFound := false
		for _, edge := range edges {
			if edge.SourceService == srv {
				// srv calls target
				// is target healthy?
				targetAffected := false
				for _, affected := range inc.AffectedServices {
					if affected == edge.TargetService {
						targetAffected = true
						break
					}
				}
				if !targetAffected {
					healthyUpstreamFound = true
					ev := evidence.Evidence{
						EvidenceID:  uuid.New().String(),
						Type:        evidence.EvidenceTypeServiceHealth,
						Timestamp:   inc.LastUpdatedAt,
						Service:     edge.TargetService,
						Description: fmt.Sprintf("%s's dependency %s remained healthy", srv, edge.TargetService),
						Source:      "RCAEngine",
					}
					e.evStore.Add(ev)
					cand.EvidenceIDs = append(cand.EvidenceIDs, ev.EvidenceID)
				}
			}
		}
		if healthyUpstreamFound {
			cand.Score += 5
			cand.Reasoning = append(cand.Reasoning, "Independent dependencies remained healthy.")
		}

		if cand.Score > 100 {
			cand.Score = 100
		}
		cand.Confidence = e.getConfidence(cand.Score)
		candidates = append(candidates, cand)
	}

	// Sort
	sort.Sort(ByScore(candidates))

	if len(candidates) > 0 {
		top := candidates[0]
		
		// Ambiguity check
		if len(candidates) > 1 {
			if top.Score - candidates[1].Score <= 5 {
				inc.RCA = &incidentmodel.RootCause{
					Service:    "AMBIGUOUS",
					Confidence: "LOW",
					Score:      0,
				}
				// We can combine top two candidates reasoning or state ambiguity
				inc.DetectionReason = fmt.Sprintf("Ambiguous root cause between %s and %s", top.Service, candidates[1].Service)
				return
			}
		}

		explanation := fmt.Sprintf("%s is the most likely root cause because: %s", top.Service, strings.Join(top.Reasoning, " "))

		inc.RCA = &incidentmodel.RootCause{
			Service:    top.Service,
			Operation:  top.Operation,
			Confidence: top.Confidence,
			Score:      top.Score,
		}
		inc.DetectionReason = explanation
		
		// Add all candidate evidence to incident to ensure completeness
		for _, eID := range top.EvidenceIDs {
			found := false
			for _, exist := range inc.EvidenceIDs {
				if exist == eID {
					found = true
					break
				}
			}
			if !found {
				inc.EvidenceIDs = append(inc.EvidenceIDs, eID)
			}
		}
		inc.Score = top.Score
		inc.Confidence = top.Confidence
	}
}
