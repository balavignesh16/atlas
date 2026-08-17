package remediation

import (
	"errors"
	"strings"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
)

var (
	ErrMissingEvidence = errors.New("action is missing evidence IDs")
	ErrUnknownEvidence = errors.New("action references unknown evidence ID")
	ErrDangerousCommand = errors.New("plan contains prohibited executable commands")
	ErrMissingSections = errors.New("plan missing required preconditions, verification, or rollback sections")
)

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) Validate(plan *RemediationPlan, inc *incidentmodel.Incident, allEvidence []*evidence.Evidence) error {
	if plan == nil {
		return errors.New("plan is nil")
	}

	// 1. Prohibited Executable Content Validation
	// The planner MUST NOT output shell commands.
	prohibited := []string{"shell", "exec", "docker", "kubectl", "powershell", "bash", "ssh"}
	
	checkProhibited := func(text string) bool {
		lower := strings.ToLower(text)
		for _, p := range prohibited {
			if strings.Contains(lower, p) {
				return true
			}
		}
		return false
	}

	for _, a := range plan.Actions {
		if checkProhibited(a.Description) || checkProhibited(string(a.Type)) {
			return ErrDangerousCommand
		}
	}
	for _, step := range plan.VerificationSteps {
		if checkProhibited(step) {
			return ErrDangerousCommand
		}
	}
	for _, rb := range plan.RollbackPlan {
		if checkProhibited(rb) {
			return ErrDangerousCommand
		}
	}

	// 2. Sections Validation
	if len(plan.Actions) == 0 {
		return errors.New("plan must have at least one action")
	}
	if len(plan.VerificationSteps) == 0 {
		return ErrMissingSections
	}
	if len(plan.Preconditions) == 0 {
		return ErrMissingSections
	}
	// Trivial plans (only OBSERVE/INVESTIGATE) might not need Rollback. 
	// But the spec says "Every non-trivial remediation plan must contain: rollbackPlan".
	if MaxRisk(plan.Actions) != RiskLow && len(plan.RollbackPlan) == 0 {
		return ErrMissingSections
	}

	// 3. Evidence Grounding
	validEv := make(map[string]bool)
	for _, e := range allEvidence {
		validEv[e.EvidenceID] = true
	}

	for _, a := range plan.Actions {
		// Even OBSERVE must carry evidence!
		if len(a.EvidenceIDs) == 0 {
			return ErrMissingEvidence
		}
		for _, eID := range a.EvidenceIDs {
			if !validEv[eID] {
				return ErrUnknownEvidence
			}
		}
	}

	// 4. Policy Engine evaluation
	rcaService := "UNKNOWN"
	rcaConfidence := "UNKNOWN"
	if inc.RCA != nil {
		rcaService = inc.RCA.Service
		rcaConfidence = inc.RCA.Confidence
	} else if inc.Status == incidentmodel.StatusOpen {
		// If M2.4 didn't produce RCA, but incident is open, it might be AMBIGUOUS
		rcaService = "AMBIGUOUS"
	}

	for _, a := range plan.Actions {
		if err := EvaluatePolicy(a, rcaService, rcaConfidence); err != nil {
			return err
		}
	}

	return nil
}
