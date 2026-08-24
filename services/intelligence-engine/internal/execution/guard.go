package execution

import (
	"errors"
	"strings"

	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

var (
	ErrExecutionDisabled       = errors.New("execution is disabled by configuration (ATLAS_EXECUTION_ENABLED=false)")
	ErrPlanNotApproved         = errors.New("plan is not in APPROVED status")
	ErrApprovalInvalid         = errors.New("approval fingerprint does not match the current plan fingerprint")
	ErrActionNotAllowlisted    = errors.New("action type is not allowlisted for execution")
	ErrMissingEvidence         = errors.New("action is missing valid evidence IDs")
	ErrServiceNotAllowlisted   = errors.New("target service is not strictly allowlisted for execution")
	ErrActionNotFound          = errors.New("action not found in plan")
)

// AllowedServices maps logical ATLAS service names to safe container/infrastructure targets.
var AllowedServices = map[string]string{
	"atlas-payment-service":   "atlas-payment-service-1",
	"atlas-order-service":     "atlas-order-service-1",
	"atlas-inventory-service": "atlas-inventory-service-1",
	"atlas-gateway":           "atlas-atlas-gateway-1",
}

type Guard struct {
	enabled bool
}

func NewGuard(enabled bool) *Guard {
	return &Guard{enabled: enabled}
}

// Check verifies that the plan is in a completely safe state to be executed.
func (g *Guard) Check(plan *remediation.RemediationPlan, actionID string) (*action.RemediationAction, error) {
	if !g.enabled {
		return nil, ErrExecutionDisabled
	}

	if plan.Status != remediation.StatusApproved {
		return nil, ErrPlanNotApproved
	}

	// 11. APPROVAL MUST be tied to the exact plan fingerprint/state.
	if plan.Fingerprint == "" || plan.Approval.ApprovedFingerprint != plan.Fingerprint {
		return nil, ErrApprovalInvalid
	}

	// Find action
	var act *action.RemediationAction
	for i := range plan.Actions {
		if plan.Actions[i].ActionID == actionID {
			act = &plan.Actions[i]
			break
		}
	}

	if act == nil {
		return nil, ErrActionNotFound
	}

	// 8. action is allowlisted (M2.6 Catalog + Execution catalog checks if it's executable)
	// We only execute specific actions like RESTART_SERVICE.
	if act.Type != action.TypeRestartService && act.Type != action.TypeObserve && act.Type != action.TypeInvestigate {
		return nil, ErrActionNotAllowlisted
	}

	// 8. service is allowlisted
	serviceTrimmed := strings.TrimSpace(act.TargetService)
	if _, ok := AllowedServices[serviceTrimmed]; !ok && act.Type != action.TypeObserve && act.Type != action.TypeInvestigate {
		return nil, ErrServiceNotAllowlisted
	}

	// 8. evidence IDs are valid
	if len(act.EvidenceIDs) == 0 {
		return nil, ErrMissingEvidence
	}

	return act, nil
}
