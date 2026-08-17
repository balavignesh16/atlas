package remediation

import (
	"testing"

	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

func TestPolicy_UnknownAction(t *testing.T) {
	err := EvaluatePolicy(action.RemediationAction{Type: action.ActionType("MAGIC_WAND")}, "atlas-payment", "HIGH")
	if err != ErrActionUnknown {
		t.Fatalf("expected ErrActionUnknown, got %v", err)
	}
}

func TestPolicy_AmbiguousHighRisk(t *testing.T) {
	err := EvaluatePolicy(action.RemediationAction{Type: action.TypeRestartService}, "AMBIGUOUS", "LOW")
	if err != ErrAmbiguousHighRisk {
		t.Fatalf("expected ErrAmbiguousHighRisk, got %v", err)
	}
}

func TestPolicy_LowConfHighRisk(t *testing.T) {
	err := EvaluatePolicy(action.RemediationAction{Type: action.TypeRestartService}, "atlas-payment", "LOW")
	if err != ErrLowConfHighRisk {
		t.Fatalf("expected ErrLowConfHighRisk, got %v", err)
	}
}

func TestPolicy_AllowedLowRiskAmbiguous(t *testing.T) {
	err := EvaluatePolicy(action.RemediationAction{Type: action.TypeObserve}, "AMBIGUOUS", "LOW")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
