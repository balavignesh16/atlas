package execution_test

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

func TestGuard_Disabled(t *testing.T) {
	g := execution.NewGuard(false)
	_, err := g.Check(&remediation.RemediationPlan{Status: remediation.StatusApproved}, "act1")
	if err != execution.ErrExecutionDisabled {
		t.Fatalf("expected ErrExecutionDisabled, got %v", err)
	}
}

func TestGuard_NotApproved(t *testing.T) {
	g := execution.NewGuard(true)
	plan := &remediation.RemediationPlan{Status: remediation.StatusProposed}
	_, err := g.Check(plan, "act1")
	if err != execution.ErrPlanNotApproved {
		t.Fatalf("expected ErrPlanNotApproved, got %v", err)
	}
}

func TestGuard_InvalidFingerprint(t *testing.T) {
	g := execution.NewGuard(true)
	plan := &remediation.RemediationPlan{
		Status:      remediation.StatusApproved,
		Fingerprint: "fingerprint-v2",
		Approval: remediation.ApprovalMetadata{
			ApprovedFingerprint: "fingerprint-v1", // mismatch!
			ApprovalReason:      "Looks good",
		},
	}
	_, err := g.Check(plan, "act1")
	if err != execution.ErrApprovalInvalid {
		t.Fatalf("expected ErrApprovalInvalid, got %v", err)
	}
}

func TestGuard_Valid(t *testing.T) {
	g := execution.NewGuard(true)
	plan := &remediation.RemediationPlan{
		Status:      remediation.StatusApproved,
		Fingerprint: "fingerprint-v1",
		Approval: remediation.ApprovalMetadata{
			ApprovedFingerprint: "fingerprint-v1",
			ApprovalReason:      "Looks good",
			ApprovedAt:          &time.Time{},
		},
		Actions: []action.RemediationAction{
			{
				ActionID:      "act1",
				Type:          action.TypeRestartService,
				TargetService: "atlas-payment-service",
				EvidenceIDs:   []string{"ev1"},
			},
		},
	}
	
	act, err := g.Check(plan, "act1")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if act.TargetService != "atlas-payment-service" {
		t.Fatalf("unexpected target service")
	}
}
