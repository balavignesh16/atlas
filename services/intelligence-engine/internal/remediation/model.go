package remediation

import (
	"context"
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

type RemediationContext struct {
	Incident    *incidentmodel.Incident
	Analysis    *aireasoning.AnalysisResult
	AllEvidence []*evidence.Evidence
}

type RemediationPlannerProvider interface {
	GeneratePlan(ctx context.Context, input *RemediationContext) (*RemediationPlan, error)
}

type PlanStatus string

const (
	StatusProposed PlanStatus = "PROPOSED"
	StatusValidated PlanStatus = "VALIDATED"
	StatusApproved PlanStatus = "APPROVED"
	StatusRejected PlanStatus = "REJECTED"
	StatusExpired  PlanStatus = "EXPIRED"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "LOW"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
)

type ApprovalMetadata struct {
	ApprovedAt          *time.Time `json:"approvedAt,omitempty"`
	RejectedAt          *time.Time `json:"rejectedAt,omitempty"`
	ApprovalReason      string     `json:"approvalReason,omitempty"`
	RejectionReason     string     `json:"rejectionReason,omitempty"`
	ApprovedFingerprint string     `json:"approvedFingerprint,omitempty"`
	// ApprovedBy is the authenticated principal name that approved this
	// plan (M2.9), populated by the HTTP layer from request-context
	// identity established by internal/security -- never from a
	// client-supplied request body field. Empty when security is disabled
	// (ATLAS_SECURITY_ENABLED=false, the default), preserving pre-M2.9
	// behavior exactly.
	ApprovedBy string `json:"approvedBy,omitempty"`
}

type RemediationPlan struct {
	PlanID            string             `json:"planId"`
	IncidentID        string             `json:"incidentId"`
	CreatedAt         time.Time          `json:"createdAt"`
	Status            PlanStatus         `json:"status"`
	RiskLevel         RiskLevel          `json:"riskLevel"`
	Confidence        string             `json:"confidence"`
	Rationale         string             `json:"rationale"`
	Preconditions     []string           `json:"preconditions"`
	Actions           []action.RemediationAction `json:"actions"`
	VerificationSteps []string           `json:"verificationSteps"`
	RollbackPlan      []string           `json:"rollbackPlan"`
	SafetyWarnings    []string           `json:"safetyWarnings"`
	EvidenceIDs       []string           `json:"evidenceIds"`
	RequiresApproval  bool               `json:"requiresApproval"`
	Planner           string             `json:"planner"`
	PlannerVersion    string             `json:"plannerVersion"`

	Approval ApprovalMetadata `json:"approval"`

	// Internal fingerprint to avoid duplicate generation
	Fingerprint string `json:"-"`
}
