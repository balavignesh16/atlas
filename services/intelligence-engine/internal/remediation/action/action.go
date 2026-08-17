package action

type ActionType string

// RemediationAction represents a completely symbolic, non-executable action.
type RemediationAction struct {
	ActionID           string            `json:"actionId"`
	Type               ActionType        `json:"type"`
	TargetService      string            `json:"targetService"`
	Description        string            `json:"description"`
	Parameters         map[string]string `json:"parameters"`
	RiskLevel          string            `json:"riskLevel"`
	RequiresApproval   bool              `json:"requiresApproval"`
	EvidenceIDs        []string          `json:"evidenceIds"`
	ExpectedOutcome    string            `json:"expectedOutcome"`
	VerificationRequired bool            `json:"verificationRequired"`
}
