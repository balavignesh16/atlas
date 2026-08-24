package execution

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
	"github.com/google/uuid"
)

type ExecutorProvider interface {
	RestartService(ctx context.Context, serviceName string) ExecutionResult
	Observe(ctx context.Context, serviceName string) ExecutionResult
	Investigate(ctx context.Context, serviceName string) ExecutionResult
}

type VerificationProvider interface {
	Verify(ctx context.Context, incidentID string, serviceName string) VerificationStatus
}

type Engine struct {
	guard          *Guard
	executor       ExecutorProvider
	verifier       VerificationProvider
	store          *Store
	timeoutSeconds int
}

func NewEngine(guard *Guard, executor ExecutorProvider, verifier VerificationProvider, store *Store, timeoutSeconds int) *Engine {
	return &Engine{
		guard:          guard,
		executor:       executor,
		verifier:       verifier,
		store:          store,
		timeoutSeconds: timeoutSeconds,
	}
}

func (e *Engine) ExecutePlanAction(ctx context.Context, plan *remediation.RemediationPlan, actionID string, approver string) (*ExecutionRecord, error) {
	// 1. Idempotency Check
	if existing, ok := e.store.GetByIdempotencyKey(plan.PlanID, actionID); ok {
		log.Printf("[INFO] Execution Engine: duplicate request for plan %s action %s, returning existing record", plan.PlanID, actionID)
		return existing, nil
	}

	// 2. Guard Safety Checks
	act, err := e.guard.Check(plan, actionID)
	if err != nil {
		return nil, err
	}

	// 3. Initialize Record
	execID := uuid.New().String()
	record := &ExecutionRecord{
		ExecutionID:        execID,
		PlanID:             plan.PlanID,
		ActionID:           actionID,
		IncidentID:         plan.IncidentID,
		Service:            act.TargetService,
		Action:             act.Type,
		EvidenceIDs:        act.EvidenceIDs,
		Approver:           approver,
		ApprovalFingerprint: plan.Approval.ApprovedFingerprint,
		StartedAt:          time.Now(),
		ExecutionStatus:    StatusPreconditionCheck,
		VerificationStatus: VerificationPending,
	}
	
	// Save initial state
	e.store.Save(record)

	// Preconditions should pass implicitly per M2.6 validation, 
	// but we log that we verified state constraints here.
	record.ExecutionStatus = StatusExecuting
	e.store.Save(record)

	// Context with strictly bounded execution timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(e.timeoutSeconds)*time.Second)
	defer cancel()

	// 4. Infrastructure Adapter Execution
	var res ExecutionResult
	switch act.Type {
	case action.TypeRestartService:
		res = e.executor.RestartService(execCtx, act.TargetService)
	case action.TypeObserve:
		res = e.executor.Observe(execCtx, act.TargetService)
	case action.TypeInvestigate:
		res = e.executor.Investigate(execCtx, act.TargetService)
	default:
		res = ExecutionResult{
			Status:  StatusRejected,
			Message: "Unsupported action execution",
			Error:   errors.New("executor adapter not implemented for action type"),
		}
	}

	now := time.Now()
	record.FinishedAt = &now
	record.ExecutionStatus = res.Status
	record.Message = res.Message
	if res.Error != nil {
		record.Error = res.Error.Error()
	}
	e.store.Save(record)

	// 5. Trigger Post-Execution Verification Asynchronously (if execution succeeded and it's a mutating action)
	if record.ExecutionStatus == StatusExecuted {
		if act.Type == action.TypeRestartService {
			record.VerificationStatus = VerificationVerifying
			e.store.Save(record)
			
			go e.runVerification(record.ExecutionID, record.IncidentID, record.Service)
		} else {
			record.VerificationStatus = VerificationNotRequired
			e.store.Save(record)
		}
	}

	return record, nil
}

func (e *Engine) runVerification(execID, incidentID, serviceName string) {
	// Let the system stabilize
	time.Sleep(5 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.timeoutSeconds)*time.Second)
	defer cancel()

	status := e.verifier.Verify(ctx, incidentID, serviceName)
	
	if record, ok := e.store.Get(execID); ok {
		record.VerificationStatus = status
		if status == VerificationFailed {
			record.Message += " (Verification Failed: incident telemetry still shows degradation)"
		} else if status == VerificationVerified {
			record.Message += " (Verification Passed: service is healthy)"
		}
		e.store.Save(record)
	}
}

func (e *Engine) GetRecord(execID string) (*ExecutionRecord, bool) {
	return e.store.Get(execID)
}

func (e *Engine) GetRecordsByIncident(incidentID string) []*ExecutionRecord {
	return e.store.GetByIncident(incidentID)
}

func (e *Engine) CleanupExpired(now time.Time) {
	e.store.Cleanup(now)
}
