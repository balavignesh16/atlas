package remediation

import (
	"testing"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

func TestValidator_ValidPlan(t *testing.T) {
	v := NewValidator()
	inc := &incidentmodel.Incident{
		IncidentID: "INC1",
		RCA: &incidentmodel.RootCause{
			Service: "atlas-payment",
			Confidence: "HIGH",
		},
	}
	evs := []*evidence.Evidence{{EvidenceID: "E1"}}

	plan := &RemediationPlan{
		Actions: []action.RemediationAction{
			{
				Type: action.TypeRestartService,
				EvidenceIDs: []string{"E1"},
			},
		},
		Preconditions: []string{"active"},
		VerificationSteps: []string{"check"},
		RollbackPlan: []string{"revert"},
	}

	if err := v.Validate(plan, inc, evs); err != nil {
		t.Fatalf("Expected valid plan, got %v", err)
	}
}

func TestValidator_MissingEvidence(t *testing.T) {
	v := NewValidator()
	inc := &incidentmodel.Incident{}
	evs := []*evidence.Evidence{{EvidenceID: "E1"}}

	plan := &RemediationPlan{
		Actions: []action.RemediationAction{
			{
				Type: action.TypeObserve,
				EvidenceIDs: []string{}, // missing!
			},
		},
		Preconditions: []string{"active"},
		VerificationSteps: []string{"check"},
	}

	err := v.Validate(plan, inc, evs)
	if err != ErrMissingEvidence {
		t.Fatalf("Expected ErrMissingEvidence, got %v", err)
	}
}

func TestValidator_UnknownEvidence(t *testing.T) {
	v := NewValidator()
	inc := &incidentmodel.Incident{}
	evs := []*evidence.Evidence{{EvidenceID: "E1"}}

	plan := &RemediationPlan{
		Actions: []action.RemediationAction{
			{
				Type: action.TypeObserve,
				EvidenceIDs: []string{"E999"}, // unknown!
			},
		},
		Preconditions: []string{"active"},
		VerificationSteps: []string{"check"},
	}

	err := v.Validate(plan, inc, evs)
	if err != ErrUnknownEvidence {
		t.Fatalf("Expected ErrUnknownEvidence, got %v", err)
	}
}

func TestValidator_PromptInjection(t *testing.T) {
	v := NewValidator()
	inc := &incidentmodel.Incident{}
	evs := []*evidence.Evidence{{EvidenceID: "E1"}}

	plan := &RemediationPlan{
		Actions: []action.RemediationAction{
			{
				Type: action.TypeObserve,
				Description: "Run shell command to fix",
				EvidenceIDs: []string{"E1"},
			},
		},
		Preconditions: []string{"active"},
		VerificationSteps: []string{"check"},
	}

	err := v.Validate(plan, inc, evs)
	if err != ErrDangerousCommand {
		t.Fatalf("Expected ErrDangerousCommand, got %v", err)
	}
}

func TestValidator_MissingRollback(t *testing.T) {
	v := NewValidator()
	inc := &incidentmodel.Incident{}
	evs := []*evidence.Evidence{{EvidenceID: "E1"}}

	plan := &RemediationPlan{
		Actions: []action.RemediationAction{
			{
				Type: action.TypeRestartService,
				EvidenceIDs: []string{"E1"},
			},
		},
		Preconditions: []string{"active"},
		VerificationSteps: []string{"check"},
		RollbackPlan: []string{}, // Missing rollback for high risk!
	}

	err := v.Validate(plan, inc, evs)
	if err != ErrMissingSections {
		t.Fatalf("Expected ErrMissingSections, got %v", err)
	}
}
