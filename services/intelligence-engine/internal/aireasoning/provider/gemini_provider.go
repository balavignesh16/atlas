package provider

import (
	"context"
	"errors"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
)

// GeminiProvider is a placeholder for the actual API integration.
// For the M2.5 milestone, we primarily focus on the architectural boundary.
type GeminiProvider struct {
	endpoint string
	model    string
}

func NewGeminiProvider(endpoint, model string) *GeminiProvider {
	return &GeminiProvider{
		endpoint: endpoint,
		model:    model,
	}
}

func (g *GeminiProvider) Analyze(ctx context.Context, input *aireasoning.IncidentAnalysisContext) (*aireasoning.AnalysisResult, error) {
	// In a real implementation, this would marshal the prompt and schema into JSON,
	// invoke the REST endpoint with the appropriate headers, parse the structured response,
	// and return the *AnalysisResult.
	// For M2.5 tests, we rely on the FakeProvider.
	return nil, errors.New("gemini provider not fully implemented in M2.5 placeholder")
}
