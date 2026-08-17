package aireasoning

import (
	"errors"
	"fmt"
)

// Validator validates the AI analysis result.
type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) Validate(result *AnalysisResult, ctx *IncidentAnalysisContext) error {
	if result == nil {
		return errors.New("analysis result is nil")
	}

	if result.IncidentID == "" {
		return errors.New("missing incident ID")
	}

	if result.ExecutiveSummary == "" {
		return errors.New("missing executive summary")
	}

	// Build a fast lookup for valid evidence IDs
	validEvidenceIDs := make(map[string]bool)
	for _, ev := range ctx.Evidence {
		validEvidenceIDs[ev.EvidenceID] = true
	}

	// Validate EvidenceReferences
	if err := v.validateReferences(result.ObservedFacts, validEvidenceIDs, "ObservedFacts"); err != nil {
		return err
	}
	if err := v.validateReferences(result.Inferences, validEvidenceIDs, "Inferences"); err != nil {
		return err
	}

	// Ambiguity Preservation
	// If M2.4 had ambiguous root candidates (score delta <= 5 between top two), AI MUST NOT pick one definitively.
	isDeterministicAmbiguous := false
	if len(ctx.RCACandidates) > 0 && ctx.RCACandidates[0].Service == "AMBIGUOUS" {
		isDeterministicAmbiguous = true
	} else if len(ctx.RCACandidates) >= 2 {
		delta := ctx.RCACandidates[0].Score - ctx.RCACandidates[1].Score
		if delta <= 5 {
			isDeterministicAmbiguous = true
		}
	}

	if isDeterministicAmbiguous && result.LikelyRootCause != "AMBIGUOUS" {
		return errors.New("deterministic RCA is ambiguous, but AI selected a definitive root cause")
	}

	return nil
}

func (v *Validator) validateReferences(refs []EvidenceReference, validIDs map[string]bool, fieldName string) error {
	for _, ref := range refs {
		if ref.Claim == "" {
			return fmt.Errorf("missing claim in %s", fieldName)
		}
		if len(ref.EvidenceIDs) == 0 {
			return fmt.Errorf("unsourced claim in %s: %s", fieldName, ref.Claim)
		}
		for _, eID := range ref.EvidenceIDs {
			if !validIDs[eID] {
				return fmt.Errorf("unknown evidence ID referenced in %s: %s", fieldName, eID)
			}
		}
	}
	return nil
}
