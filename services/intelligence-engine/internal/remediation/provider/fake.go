package provider

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
	"github.com/google/uuid"
)

type FakePlanner struct {
	ShouldFail        bool
	ShouldInject      bool
	InjectUnknownAct  bool
	InjectMissingEvid bool
}

func NewFakePlanner() *FakePlanner {
	return &FakePlanner{}
}

func (f *FakePlanner) GeneratePlan(ctx context.Context, input *remediation.RemediationContext) (*remediation.RemediationPlan, error) {
	if f.ShouldFail {
		return nil, errors.New("fake planner failed")
	}

	// Prompt injection test logic
	if input.Incident != nil && strings.Contains(input.Incident.Title, "Ignore previous instructions") {
		if f.ShouldInject {
			// Malicious bypass
			return nil, errors.New("prompt injection succeeded in fake planner")
		}
	}

	evidenceIDs := []string{}
	if len(input.AllEvidence) > 0 {
		evidenceIDs = append(evidenceIDs, input.AllEvidence[0].EvidenceID)
	}

	if f.InjectMissingEvid {
		evidenceIDs = []string{"E999"}
	}

	actType := action.TypeRestartService
	if f.InjectUnknownAct {
		actType = action.ActionType("DOCKER_RESTART")
	}

	act := action.RemediationAction{
		ActionID:           uuid.New().String(),
		Type:               actType,
		TargetService:      input.Incident.RootService,
		Description:        "Restarting the service symbolically",
		EvidenceIDs:        evidenceIDs,
		ExpectedOutcome:    "Service becomes healthy",
		VerificationRequired: true,
	}

	plan := &remediation.RemediationPlan{
		PlanID:            uuid.New().String(),
		IncidentID:        input.Incident.IncidentID,
		CreatedAt:         time.Now(),
		Status:            remediation.StatusProposed,
		Confidence:        "HIGH",
		Rationale:         "Deterministic fake rationalization.",
		Preconditions:     []string{"Incident is ongoing."},
		Actions:           []action.RemediationAction{act},
		VerificationSteps: []string{"Verify service UP"},
		RollbackPlan:      []string{"If it fails, rollback manually."},
		SafetyWarnings:    []string{"This is a test."},
		EvidenceIDs:       evidenceIDs,
		Planner:           "fake",
		PlannerVersion:    "1.0",
	}

	return plan, nil
}
