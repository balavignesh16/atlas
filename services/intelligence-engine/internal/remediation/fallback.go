package remediation

import (
	"context"
	"time"

	"github.com/atlas/intelligence-engine/internal/remediation/action"
	"github.com/google/uuid"
)

type FallbackPlanner struct{}

func NewFallbackPlanner() *FallbackPlanner {
	return &FallbackPlanner{}
}

func (f *FallbackPlanner) GeneratePlan(ctx context.Context, input *RemediationContext) (*RemediationPlan, error) {
	
	evidenceIDs := []string{}
	for _, ev := range input.AllEvidence {
		evidenceIDs = append(evidenceIDs, ev.EvidenceID)
	}

	act := action.RemediationAction{
		ActionID:           uuid.New().String(),
		Type:               action.TypeObserve,
		TargetService:      "UNKNOWN",
		Description:        "Fallback diagnostic observation.",
		EvidenceIDs:        evidenceIDs,
		ExpectedOutcome:    "Understanding of incident.",
		VerificationRequired: true,
	}

	if input.Incident.RCA != nil && input.Incident.RCA.Service != "AMBIGUOUS" {
		act.TargetService = input.Incident.RCA.Service
		act.Type = action.TypeRestartService
		act.Description = "Deterministic fallback restart."
	}

	plan := &RemediationPlan{
		PlanID:            uuid.New().String(),
		IncidentID:        input.Incident.IncidentID,
		CreatedAt:         time.Now(),
		Status:            StatusProposed,
		Confidence:        "MEDIUM",
		Rationale:         "Fallback deterministic logic triggered.",
		Preconditions:     []string{"Incident active."},
		Actions:           []action.RemediationAction{act},
		VerificationSteps: []string{"Verify health."},
		RollbackPlan:      []string{"Abort actions."},
		SafetyWarnings:    []string{"Fallback generated plan."},
		EvidenceIDs:       evidenceIDs,
		Planner:           "fallback",
		PlannerVersion:    "1.0",
	}

	return plan, nil
}
