package remediation

import (
	"testing"

	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

func TestRisk_EvaluateRisk(t *testing.T) {
	if EvaluateRisk(action.RemediationAction{Type: action.TypeObserve}) != RiskLow {
		t.Fatal("Expected LOW")
	}
	if EvaluateRisk(action.RemediationAction{Type: action.TypeReduceTraffic}) != RiskMedium {
		t.Fatal("Expected MEDIUM")
	}
	if EvaluateRisk(action.RemediationAction{Type: action.TypeRestartService}) != RiskHigh {
		t.Fatal("Expected HIGH")
	}
	if EvaluateRisk(action.RemediationAction{Type: action.TypeRollbackDeployment}) != RiskCritical {
		t.Fatal("Expected CRITICAL")
	}
	if EvaluateRisk(action.RemediationAction{Type: action.ActionType("UNKNOWN")}) != RiskCritical {
		t.Fatal("Expected CRITICAL for unknown")
	}
}

func TestRisk_MaxRisk(t *testing.T) {
	actions := []action.RemediationAction{
		{Type: action.TypeObserve},
		{Type: action.TypeRestartService},
		{Type: action.TypeReduceTraffic},
	}
	if MaxRisk(actions) != RiskHigh {
		t.Fatal("Expected HIGH max risk")
	}
}
