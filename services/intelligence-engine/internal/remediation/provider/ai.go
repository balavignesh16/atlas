package provider

import (
	"context"
	"errors"

	"github.com/atlas/intelligence-engine/internal/remediation"
)

// AIPlanner is a placeholder for external generative API.
type AIPlanner struct {
	endpoint string
	model    string
}

func NewAIPlanner(endpoint, model string) *AIPlanner {
	return &AIPlanner{
		endpoint: endpoint,
		model:    model,
	}
}

func (a *AIPlanner) GeneratePlan(ctx context.Context, input *remediation.RemediationContext) (*remediation.RemediationPlan, error) {
	// M2.6 relies on Fake/Fallback for testing until full generative implementation.
	return nil, errors.New("AI Planner not fully implemented in M2.6 placeholder")
}
