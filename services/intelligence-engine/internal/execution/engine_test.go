package execution_test

import (
	"context"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/atlas/intelligence-engine/internal/execution/provider"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

func TestEngine_Execution(t *testing.T) {
	guard := execution.NewGuard(true)
	store := execution.NewStore(3600)
	fakeExec := provider.NewFakeExecutor()
	
	engine := execution.NewEngine(guard, fakeExec, nil, store, 5)

	plan := &remediation.RemediationPlan{
		PlanID:      "plan1",
		IncidentID:  "inc1",
		Status:      remediation.StatusApproved,
		Fingerprint: "fp1",
		Approval: remediation.ApprovalMetadata{
			ApprovedFingerprint: "fp1",
			ApprovalReason:      "OK",
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

	record, err := engine.ExecutePlanAction(context.Background(), plan, "act1", "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if record.ExecutionStatus != execution.StatusExecuted {
		t.Fatalf("expected status EXECUTED, got %s", record.ExecutionStatus)
	}

	// Idempotency test
	record2, err := engine.ExecutePlanAction(context.Background(), plan, "act1", "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	if record2.ExecutionID != record.ExecutionID {
		t.Fatalf("expected idempotency to return the same record")
	}
}
