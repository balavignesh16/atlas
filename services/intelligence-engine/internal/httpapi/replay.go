package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/replay"
)

// ReplayAPI exposes Module 5's read-only simulation capability. There is no
// endpoint anywhere in this file (or reachable from it) that approves or
// executes a replay-generated plan -- internal/execution is never imported
// by this file, internal/replay, or anything internal/replay depends on.
type ReplayAPI struct {
	simulator *replay.Simulator
}

func NewReplayAPI(simulator *replay.Simulator) *ReplayAPI {
	return &ReplayAPI{simulator: simulator}
}

type historicalRCAView struct {
	Available       bool   `json:"available"`
	Service         string `json:"service,omitempty"`
	Confidence      string `json:"confidence,omitempty"`
	Score           int    `json:"score,omitempty"`
	DetectionReason string `json:"detectionReason,omitempty"`
}

type evidenceContextView struct {
	Requested int `json:"requested"`
	Found     int `json:"found"`
}

type dependencyContextView struct {
	EdgeCount int `json:"edgeCount"`
}

type aiReplayView struct {
	Attempted bool                        `json:"attempted"`
	Succeeded bool                        `json:"succeeded"`
	Reason    string                      `json:"reason,omitempty"`
	Result    *aireasoning.AnalysisResult `json:"result,omitempty"`
}

type planReplayView struct {
	Attempted bool                         `json:"attempted"`
	Succeeded bool                         `json:"succeeded"`
	Reason    string                       `json:"reason,omitempty"`
	Plan      *remediation.RemediationPlan `json:"plan,omitempty"`
}

// replayResponse's Simulation/ExecutionPerformed/ApprovalPerformed fields
// are always true/false/false -- present explicitly (never omitted) so a
// caller can never mistake this response for a real execution result by
// the absence of a field.
type replayResponse struct {
	ReplayID           string    `json:"replayId"`
	SourceIncidentID   string    `json:"sourceIncidentId"`
	ReplayTimestamp    time.Time `json:"replayTimestamp"`
	Simulation         bool      `json:"simulation"`
	ExecutionPerformed bool      `json:"executionPerformed"`
	ApprovalPerformed  bool      `json:"approvalPerformed"`

	HistoricalRCA historicalRCAView     `json:"historicalRCA"`
	Evidence      evidenceContextView   `json:"evidence"`
	Dependencies  dependencyContextView `json:"dependencies"`

	ReplayAnalysis     aiReplayView                `json:"replayAnalysis"`
	HistoricalAnalysis *aireasoning.AnalysisResult `json:"historicalAnalysis,omitempty"`

	ReplayPlan     planReplayView               `json:"replayPlan"`
	HistoricalPlan *remediation.RemediationPlan `json:"historicalPlan,omitempty"`
}

func toReplayResponse(r replay.Result) replayResponse {
	return replayResponse{
		ReplayID:           r.ReplayID,
		SourceIncidentID:   r.SourceIncidentID,
		ReplayTimestamp:    r.ReplayTimestamp,
		Simulation:         r.Simulation,
		ExecutionPerformed: r.ExecutionPerformed,
		ApprovalPerformed:  r.ApprovalPerformed,
		HistoricalRCA: historicalRCAView{
			Available:       r.HistoricalRCA.Available,
			Service:         r.HistoricalRCA.Service,
			Confidence:      r.HistoricalRCA.Confidence,
			Score:           r.HistoricalRCA.Score,
			DetectionReason: r.HistoricalRCA.DetectionReason,
		},
		Evidence:     evidenceContextView{Requested: r.Evidence.Requested, Found: r.Evidence.Found},
		Dependencies: dependencyContextView{EdgeCount: r.Dependencies.EdgeCount},
		ReplayAnalysis: aiReplayView{
			Attempted: r.ReplayAnalysis.Attempted,
			Succeeded: r.ReplayAnalysis.Succeeded,
			Reason:    r.ReplayAnalysis.Reason,
			Result:    r.ReplayAnalysis.Result,
		},
		HistoricalAnalysis: r.HistoricalAnalysis,
		ReplayPlan: planReplayView{
			Attempted: r.ReplayPlan.Attempted,
			Succeeded: r.ReplayPlan.Succeeded,
			Reason:    r.ReplayPlan.Reason,
			Plan:      r.ReplayPlan.Plan,
		},
		HistoricalPlan: r.HistoricalPlan,
	}
}

// HandleReplayIncident handles POST /api/v1/incidents/{id}/replay. 404 only
// when the incident is entirely unknown to the manager; every other honest
// outcome (no RCA yet, no evidence left, AI/plan generation declining) is a
// real 200 -- see replay.Result's own field-level Attempted/Succeeded/Reason
// semantics, matching this project's established "partial evidence is not
// an error" convention (Phase 7D's serviceintel, Phase 7C's registry).
func (a *ReplayAPI) HandleReplayIncident(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result, ok, err := a.simulator.Simulate(r.Context(), id, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REPLAY_ERROR", "Failed to simulate incident replay")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "INCIDENT_NOT_FOUND", "No such incident is known (never existed, or already cleaned up past its retention window)")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toReplayResponse(result))
}
