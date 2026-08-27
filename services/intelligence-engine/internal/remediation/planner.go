package remediation

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

type Config struct {
	Enabled          bool
	Provider         string
	RetentionSeconds int
}

type Planner struct {
	cfg       Config
	provider  RemediationPlannerProvider
	fallback  RemediationPlannerProvider
	validator *Validator
	store     *Store
	
	fingerprintCache map[string]string // incidentId -> fingerprint
}

func NewPlanner(cfg Config, prov RemediationPlannerProvider) *Planner {
	return &Planner{
		cfg:              cfg,
		provider:         prov,
		fallback:         NewFallbackPlanner(),
		validator:        NewValidator(),
		store:            NewStore(cfg.RetentionSeconds),
		fingerprintCache: make(map[string]string),
	}
}

func (p *Planner) GeneratePlan(ctx context.Context, incident *incidentmodel.Incident, analysis *aireasoning.AnalysisResult, allEvidence []*evidence.Evidence, force bool) (*RemediationPlan, error) {
	
	fingerprint := GenerateFingerprint(incident, analysis)
	
	if !force {
		lastFp, ok := p.fingerprintCache[incident.IncidentID]
		if ok && lastFp == fingerprint {
			if existing, found := p.store.GetByIncident(incident.IncidentID); found {
				return existing, nil
			}
		}
	}

	reqCtx := &RemediationContext{
		Incident:    incident,
		Analysis:    analysis,
		AllEvidence: allEvidence,
	}

	var plan *RemediationPlan
	var err error

	if p.cfg.Enabled {
		plan, err = p.provider.GeneratePlan(ctx, reqCtx)
	}

	// Fallback logic
	if !p.cfg.Enabled || err != nil {
		if err != nil {
			log.Printf("[WARN] AI Planner failed: %v. Falling back to deterministic planner.", err)
		}
		plan, err = p.fallback.GeneratePlan(ctx, reqCtx)
		if err != nil {
			return nil, fmt.Errorf("fallback planner failed: %w", err)
		}
	}

	// Validate Plan
	if err := p.validator.Validate(plan, incident, allEvidence); err != nil {
		return nil, fmt.Errorf("plan validation failed: %w", err)
	}

	plan.Fingerprint = fingerprint
	plan.RiskLevel = MaxRisk(plan.Actions)
	plan.Status = StatusValidated
	
	p.fingerprintCache[incident.IncidentID] = fingerprint
	p.store.Save(plan)

	return plan, nil
}

// approvedBy is the authenticated principal name (from internal/security's
// request-context identity, resolved at the HTTP layer) that approved this
// plan -- never a client-supplied request body value. Empty when security
// is disabled, matching pre-M2.9 behavior.
func (p *Planner) ApprovePlan(planID string, reason string, approvedBy string) (*RemediationPlan, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()

	plan, ok := p.store.plans[planID]
	if !ok {
		return nil, fmt.Errorf("plan not found")
	}

	if plan.Status != StatusProposed && plan.Status != StatusValidated {
		return nil, fmt.Errorf("cannot approve plan in status: %s", plan.Status)
	}

	now := time.Now()
	plan.Status = StatusApproved
	plan.Approval = ApprovalMetadata{
		ApprovedAt:          &now,
		ApprovalReason:      reason,
		ApprovedFingerprint: plan.Fingerprint,
		ApprovedBy:          approvedBy,
	}

	// Safe copy
	pCopy := *plan
	return &pCopy, nil
}

func (p *Planner) RejectPlan(planID string, reason string) (*RemediationPlan, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()

	plan, ok := p.store.plans[planID]
	if !ok {
		return nil, fmt.Errorf("plan not found")
	}

	if plan.Status != StatusProposed && plan.Status != StatusValidated {
		return nil, fmt.Errorf("cannot reject plan in status: %s", plan.Status)
	}

	now := time.Now()
	plan.Status = StatusRejected
	plan.Approval = ApprovalMetadata{
		RejectedAt:      &now,
		RejectionReason: reason,
	}

	// Safe copy
	pCopy := *plan
	return &pCopy, nil
}

func (p *Planner) GetPlan(planID string) (*RemediationPlan, bool) {
	return p.store.Get(planID)
}

func (p *Planner) GetPlanByIncident(incidentID string) (*RemediationPlan, bool) {
	return p.store.GetByIncident(incidentID)
}

func (p *Planner) CleanupExpired(now time.Time) {
	p.store.Cleanup(now)
}
