package aireasoning

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/rca"
)

type MockProvider struct {
	Result *AnalysisResult
	Err    error
}

func (m *MockProvider) Analyze(ctx context.Context, input *IncidentAnalysisContext) (*AnalysisResult, error) {
	return m.Result, m.Err
}

func TestEngine_Analyze_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	engine := NewEngine(cfg, &MockProvider{})

	_, err := engine.Analyze(&incidentmodel.Incident{}, nil, nil, nil, nil, false)
	if !errors.Is(err, ErrDisabled) {
		t.Errorf("expected ErrDisabled, got %v", err)
	}
}

func TestEngine_Analyze_AmbiguousValidation(t *testing.T) {
	cfg := Config{Enabled: true, MaxEvents: 10}

	// M2.4 provides ambiguous RCA
	candidates := []*rca.RCACandidate{
		{Service: "ServiceA", Score: 90},
		{Service: "ServiceB", Score: 88},
	}

	prov := &MockProvider{
		Result: &AnalysisResult{
			IncidentID:       "123",
			ExecutiveSummary: "summary",
			// AI erroneously picks ServiceA definitively!
			LikelyRootCause: "ServiceA",
			ObservedFacts:   []EvidenceReference{{Claim: "x", EvidenceIDs: []string{"E1"}}},
		},
	}
	engine := NewEngine(cfg, prov)

	inc := &incidentmodel.Incident{IncidentID: "123"}
	ev := []*evidence.Evidence{{EvidenceID: "E1"}}

	_, err := engine.Analyze(inc, nil, ev, candidates, nil, false)
	if err == nil {
		t.Errorf("expected validation error for overriding ambiguity")
	}
}

func TestEngine_Analyze_Valid(t *testing.T) {
	cfg := Config{Enabled: true, MaxEvents: 10}

	prov := &MockProvider{
		Result: &AnalysisResult{
			IncidentID:       "123",
			ExecutiveSummary: "summary",
			LikelyRootCause:  "ServiceA",
			ObservedFacts:    []EvidenceReference{{Claim: "x", EvidenceIDs: []string{"E1"}}},
		},
	}
	engine := NewEngine(cfg, prov)

	inc := &incidentmodel.Incident{IncidentID: "123", LastUpdatedAt: time.Now()}
	ev := []*evidence.Evidence{{EvidenceID: "E1"}}
	candidates := []*rca.RCACandidate{{Service: "ServiceA", Score: 90}}

	res, err := engine.Analyze(inc, nil, ev, candidates, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.LikelyRootCause != "ServiceA" {
		t.Errorf("expected ServiceA")
	}

	// Test caching/fingerprint
	prov.Err = errors.New("should not be called")
	_, err = engine.Analyze(inc, nil, ev, candidates, nil, false)
	if err != nil {
		t.Errorf("expected cached result, got error: %v", err)
	}
}

func TestEngine_Analyze_ProviderFailure(t *testing.T) {
	cfg := Config{Enabled: true, MaxEvents: 10}
	prov := &MockProvider{Err: errors.New("provider down")}
	engine := NewEngine(cfg, prov)

	_, err := engine.Analyze(&incidentmodel.Incident{}, nil, nil, nil, nil, false)
	if err == nil {
		t.Errorf("expected provider error")
	}
}
