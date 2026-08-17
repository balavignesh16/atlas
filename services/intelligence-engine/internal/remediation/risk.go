package remediation

import "github.com/atlas/intelligence-engine/internal/remediation/action"

// EvaluateRisk categorizes an action into RiskLow, RiskMedium, RiskHigh, or RiskCritical.
func EvaluateRisk(act action.RemediationAction) RiskLevel {
	switch act.Type {
	case action.TypeObserve, action.TypeInvestigate:
		return RiskLow
	case action.TypeReduceTraffic, action.TypeRestoreTraffic, action.TypeClearConnectionPool:
		return RiskMedium
	case action.TypeRestartService, action.TypeScaleService:
		return RiskHigh
	case action.TypeRollbackDeployment:
		return RiskCritical
	default:
		// Unknown action defaults to CRITICAL risk for safety.
		return RiskCritical
	}
}

// MaxRisk returns the highest risk level among a list of actions.
func MaxRisk(actions []action.RemediationAction) RiskLevel {
	var current RiskLevel = RiskLow
	
	weight := map[RiskLevel]int{
		RiskLow:      1,
		RiskMedium:   2,
		RiskHigh:     3,
		RiskCritical: 4,
	}

	for _, act := range actions {
		r := EvaluateRisk(act)
		if weight[r] > weight[current] {
			current = r
		}
	}
	return current
}
