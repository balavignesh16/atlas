package remediation

import (
	"errors"

	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

var (
	ErrActionUnknown      = errors.New("action type is unknown or not allowed")
	ErrAmbiguousHighRisk  = errors.New("cannot perform HIGH/CRITICAL action on AMBIGUOUS RCA")
	ErrLowConfHighRisk    = errors.New("cannot perform HIGH/CRITICAL action on LOW confidence RCA")
)

// EvaluatePolicy checks if a proposed action violates strict safety thresholds.
func EvaluatePolicy(act action.RemediationAction, rcaService string, rcaConfidence string) error {
	if !action.IsValid(act.Type) {
		return ErrActionUnknown
	}

	risk := EvaluateRisk(act)

	if rcaService == "AMBIGUOUS" {
		if risk == RiskHigh || risk == RiskCritical {
			return ErrAmbiguousHighRisk
		}
	}

	if rcaConfidence == "LOW" {
		if risk == RiskHigh || risk == RiskCritical {
			return ErrLowConfHighRisk
		}
	}

	return nil
}
