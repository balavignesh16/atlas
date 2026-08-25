package execution_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/atlas/intelligence-engine/internal/execution/provider"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

// fakeVerifier is a minimal, test-local VerificationProvider. Passing a
// literal nil verifier here used to rely on the old runVerification's
// unconditional 5s pre-sleep to outlast the test process before the
// resulting nil-pointer dereference could be reached; M2.7.4 removed that
// blind sleep, so a nil verifier now panics near-immediately in the
// verification goroutine. This fake replaces that accidental reliance on
// timing with a real, deterministic implementation.
type fakeVerifier struct {
	status execution.VerificationStatus
	delay  time.Duration
	calls  int32
}

func (f *fakeVerifier) Verify(ctx context.Context, incidentID, serviceName string, executionFinishedAt time.Time) execution.VerificationStatus {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
		}
	}
	return f.status
}

func newTestPlan(planID, incidentID, actionID string) *remediation.RemediationPlan {
	return &remediation.RemediationPlan{
		PlanID:      planID,
		IncidentID:  incidentID,
		Status:      remediation.StatusApproved,
		Fingerprint: "fp1",
		Approval: remediation.ApprovalMetadata{
			ApprovedFingerprint: "fp1",
			ApprovalReason:      "OK",
			ApprovedAt:          &time.Time{},
		},
		Actions: []action.RemediationAction{
			{
				ActionID:      actionID,
				Type:          action.TypeRestartService,
				TargetService: "atlas-payment-service",
				EvidenceIDs:   []string{"ev1"},
			},
		},
	}
}

func TestEngine_Execution(t *testing.T) {
	guard := execution.NewGuard(true)
	store := execution.NewStore(3600)
	fakeExec := provider.NewFakeExecutor()
	verifier := &fakeVerifier{status: execution.VerificationVerified}

	engine := execution.NewEngine(guard, fakeExec, verifier, store, 5)

	plan := newTestPlan("plan1", "inc1", "act1")

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

// 1. Execution failure -> verification must never start.
func TestEngine_ExecutionFailure_VerificationNeverStarts(t *testing.T) {
	guard := execution.NewGuard(true)
	store := execution.NewStore(3600)
	fakeExec := &provider.FakeExecutor{ShouldFail: true}
	verifier := &fakeVerifier{status: execution.VerificationVerified}

	engine := execution.NewEngine(guard, fakeExec, verifier, store, 5)
	plan := newTestPlan("plan-fail", "inc-fail", "act-fail")

	record, err := engine.ExecutePlanAction(context.Background(), plan, "act-fail", "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record.ExecutionStatus != execution.StatusFailed {
		t.Fatalf("expected execution status FAILED, got %s", record.ExecutionStatus)
	}
	if record.VerificationStatus != execution.VerificationPending {
		t.Fatalf("expected verification to never start on execution failure, got %s", record.VerificationStatus)
	}

	// Give a would-be verification goroutine a chance to run; it must not.
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt32(&verifier.calls) != 0 {
		t.Fatalf("expected Verify to never be called after an execution failure, was called %d time(s)", verifier.calls)
	}
}

// 8/12. Repeated GetRecord calls while verification is in-flight return
// stable, non-mutating snapshots, and concurrent reads/writes are race-safe.
func TestEngine_ConcurrentGetRecordDuringVerification_NoRace(t *testing.T) {
	guard := execution.NewGuard(true)
	store := execution.NewStore(3600)
	fakeExec := provider.NewFakeExecutor()
	verifier := &fakeVerifier{status: execution.VerificationVerified, delay: 300 * time.Millisecond}

	engine := execution.NewEngine(guard, fakeExec, verifier, store, 5)
	plan := newTestPlan("plan-race", "inc-race", "act-race")

	record, err := engine.ExecutePlanAction(context.Background(), plan, "act-race", "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			deadline := time.Now().Add(500 * time.Millisecond)
			for time.Now().Before(deadline) {
				if _, ok := engine.GetRecord(record.ExecutionID); !ok {
					t.Error("expected the record to always be readable during verification")
					return
				}
			}
		}()
	}
	wg.Wait()

	final, ok := engine.GetRecord(record.ExecutionID)
	if !ok {
		t.Fatalf("expected final record to exist")
	}
	if final.VerificationStatus != execution.VerificationVerified {
		t.Fatalf("expected VERIFIED once verification settles, got %s", final.VerificationStatus)
	}
	if atomic.LoadInt32(&verifier.calls) != 1 {
		t.Fatalf("expected Verify to be called exactly once, got %d", verifier.calls)
	}
}
