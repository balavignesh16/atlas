package execution

import (
	"time"

	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

type ExecutionStatus string

const (
	StatusPending            ExecutionStatus = "PENDING"
	StatusPreconditionCheck  ExecutionStatus = "PRECONDITION_CHECK"
	StatusExecuting          ExecutionStatus = "EXECUTING"
	StatusExecuted           ExecutionStatus = "EXECUTED"
	StatusFailed             ExecutionStatus = "FAILED"
	StatusDisabled           ExecutionStatus = "DISABLED"
	StatusRejected           ExecutionStatus = "REJECTED"
)

type VerificationStatus string

const (
	VerificationPending VerificationStatus = "PENDING"
	VerificationVerifying VerificationStatus = "VERIFYING"
	VerificationVerified VerificationStatus = "VERIFIED"
	// VerificationFailed means a genuinely observed matching error event for
	// the target service occurred after the remediation execution finished.
	// It is not triggered merely by Incident.LastUpdatedAt advancing or by
	// repeated evaluation of stale rolling-window data.
	VerificationFailed   VerificationStatus = "FAILED"
	// VerificationTimeout means the deadline was reached with no positive
	// confirmation of recovery and no positive evidence of continued
	// failure either -- the incident may still recover shortly after this
	// point. Distinct from VerificationFailed on purpose: absence of
	// confirmation is not evidence of failure.
	VerificationTimeout  VerificationStatus = "VERIFICATION_TIMEOUT"
	VerificationNotRequired VerificationStatus = "NOT_REQUIRED"
)

type ExecutionRecord struct {
	ExecutionID        string             `json:"executionId"`
	PlanID             string             `json:"planId"`
	ActionID           string             `json:"actionId"`
	IncidentID         string             `json:"incidentId"`
	Service            string             `json:"service"`
	Action             action.ActionType  `json:"action"`
	EvidenceIDs        []string           `json:"evidenceIds"`
	Approver           string             `json:"approver"`
	ApprovalFingerprint string            `json:"approvalFingerprint"`
	StartedAt          time.Time          `json:"startedAt"`
	FinishedAt         *time.Time         `json:"finishedAt,omitempty"`
	ExecutionStatus    ExecutionStatus    `json:"executionStatus"`
	VerificationStatus VerificationStatus `json:"verificationStatus"`
	Message            string             `json:"message,omitempty"`
	Error              string             `json:"error,omitempty"`
}

type ExecutionResult struct {
	Status  ExecutionStatus
	Message string
	Error   error
}
