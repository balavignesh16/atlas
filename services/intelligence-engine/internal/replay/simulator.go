package replay

import (
	"context"
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/rca"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/google/uuid"
)

// Simulator composes a Result from existing, already-running components. It
// holds two kinds of references, deliberately kept separate:
//
//   - manager/evStore/depGraph, and the production aiEngine/rmPlanner --
//     read-only sources of real, already-persisted state (the incident
//     itself, its evidence, the live graph, and whatever historical
//     AI/plan output production already generated for it, if any).
//   - aiCfg/aiProvider and rmCfg/rmProvider -- the raw configuration and
//     provider objects production itself was constructed from. Simulate
//     wraps these in a brand-new aireasoning.Engine/remediation.Planner on
//     every call, so replay's own AI/plan invocation writes only into that
//     call's own throwaway instance, never into the shared production
//     store aiEngine/rmPlanner actually serve real requests from. The
//     instance is discarded when the request completes -- no replay-
//     specific persistence is introduced anywhere.
type Simulator struct {
	manager  *incidentmanager.Manager
	evStore  *evidence.Store
	depGraph *graph.DependencyGraph

	aiEngine *aireasoning.Engine
	aiCfg    aireasoning.Config
	aiProv   aireasoning.ReasoningProvider

	rmPlanner *remediation.Planner
	rmCfg     remediation.Config
	rmProv    remediation.RemediationPlannerProvider
}

func NewSimulator(
	manager *incidentmanager.Manager,
	evStore *evidence.Store,
	depGraph *graph.DependencyGraph,
	aiEngine *aireasoning.Engine,
	aiCfg aireasoning.Config,
	aiProv aireasoning.ReasoningProvider,
	rmPlanner *remediation.Planner,
	rmCfg remediation.Config,
	rmProv remediation.RemediationPlannerProvider,
) *Simulator {
	return &Simulator{
		manager:   manager,
		evStore:   evStore,
		depGraph:  depGraph,
		aiEngine:  aiEngine,
		aiCfg:     aiCfg,
		aiProv:    aiProv,
		rmPlanner: rmPlanner,
		rmCfg:     rmCfg,
		rmProv:    rmProv,
	}
}

// Simulate replays incidentID against currently-available persisted
// context. ok is false only when the incident is entirely unknown to the
// manager (never found, or already cleaned up past its retention window --
// this package cannot distinguish the two, since incidentmanager itself
// does not). now is used verbatim for ReplayTimestamp; Simulate never reads
// the wall clock itself beyond what its callees (aireasoning.Engine,
// remediation.Planner) already do for their own GeneratedAt/CreatedAt
// fields -- see the package doc for why full byte-identical determinism
// cannot be claimed for those.
func (s *Simulator) Simulate(ctx context.Context, incidentID string, now time.Time) (Result, bool, error) {
	inc := s.manager.GetIncident(incidentID)
	if inc == nil {
		return Result{}, false, nil
	}

	result := Result{
		ReplayID:           uuid.New().String(),
		SourceIncidentID:   incidentID,
		ReplayTimestamp:    now,
		Simulation:         true,
		ExecutionPerformed: false,
		ApprovalPerformed:  false,
	}

	if inc.RCA != nil {
		result.HistoricalRCA = HistoricalRCA{
			Available:       true,
			Service:         inc.RCA.Service,
			Confidence:      inc.RCA.Confidence,
			Score:           inc.RCA.Score,
			DetectionReason: inc.DetectionReason,
		}
	}

	evs := s.evStore.GetAll(inc.EvidenceIDs)
	evidences := make([]*evidence.Evidence, len(evs))
	for i := range evs {
		evidences[i] = &evs[i]
	}
	result.Evidence = EvidenceContext{
		Requested: len(inc.EvidenceIDs),
		Found:     len(evidences),
		Evidence:  evidences,
	}

	edges := s.depGraph.GetEdges()
	result.Dependencies = DependencyContext{
		EdgeCount: len(edges),
		Edges:     edges,
	}

	// Mirrors internal/httpapi's own HandlePostAnalyze reconstruction
	// exactly -- neither this package nor the real production endpoint has
	// access to rca.Engine's original, transient multi-candidate
	// evaluation; both can only reconstruct a single winning (or
	// AMBIGUOUS) candidate from the incident's own already-persisted RCA.
	var candidates []*rca.RCACandidate
	if inc.RCA != nil {
		candidates = []*rca.RCACandidate{
			{Service: inc.RCA.Service, Score: inc.RCA.Score, Confidence: inc.RCA.Confidence},
		}
	}

	freshAIEngine := aireasoning.NewEngine(s.aiCfg, s.aiProv)
	aiResult, aiErr := freshAIEngine.Analyze(inc, nil, evidences, candidates, edges, true)
	if aiErr != nil {
		result.ReplayAnalysis = AIReplayOutcome{Attempted: true, Succeeded: false, Reason: aiErr.Error()}
	} else {
		result.ReplayAnalysis = AIReplayOutcome{Attempted: true, Succeeded: true, Result: aiResult}
	}
	if historicalAnalysis, ok := s.aiEngine.GetAnalysis(incidentID); ok {
		result.HistoricalAnalysis = historicalAnalysis
	}

	var replayAnalysisForPlan *aireasoning.AnalysisResult
	if result.ReplayAnalysis.Succeeded {
		replayAnalysisForPlan = result.ReplayAnalysis.Result
	}
	freshPlanner := remediation.NewPlanner(s.rmCfg, s.rmProv)
	plan, planErr := freshPlanner.GeneratePlan(ctx, inc, replayAnalysisForPlan, evidences, true)
	if planErr != nil {
		result.ReplayPlan = PlanReplayOutcome{Attempted: true, Succeeded: false, Reason: planErr.Error()}
	} else {
		result.ReplayPlan = PlanReplayOutcome{Attempted: true, Succeeded: true, Plan: plan}
	}
	if historicalPlan, ok := s.rmPlanner.GetPlanByIncident(incidentID); ok {
		result.HistoricalPlan = historicalPlan
	}

	return result, true, nil
}
