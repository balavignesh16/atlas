package aireasoning

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/rca"
)

var (
	ErrDisabled         = errors.New("ai reasoning is disabled")
	ErrUnchanged        = errors.New("incident state unchanged, skipping duplicate analysis")
	ErrValidationFailed = errors.New("ai analysis failed validation")
)

// Engine orchestrates the M2.5 AI reasoning flow.
type Engine struct {
	cfg       Config
	provider  ReasoningProvider
	builder   *Builder
	validator *Validator
	store     *Store

	// fingerprintCache stores the last analyzed fingerprint for an incident
	fingerprintCache map[string]string
}

func NewEngine(cfg Config, prov ReasoningProvider) *Engine {
	return &Engine{
		cfg:              cfg,
		provider:         prov,
		builder:          NewBuilder(cfg),
		validator:        NewValidator(),
		store:            NewStore(cfg.RetentionSeconds),
		fingerprintCache: make(map[string]string),
	}
}

// Analyze triggers an AI analysis. Returns the resulting AnalysisResult or an error.
func (e *Engine) Analyze(
	incident *incidentmodel.Incident,
	signals []incidentsignal.Signal,
	evidences []*evidence.Evidence,
	candidates []*rca.RCACandidate,
	edges []*correlationmodel.DependencyEdge,
	force bool,
) (*AnalysisResult, error) {

	if !e.cfg.Enabled {
		return nil, ErrDisabled
	}

	fingerprint := GenerateFingerprint(incident)
	if !force {
		lastFp, ok := e.fingerprintCache[incident.IncidentID]
		if ok && lastFp == fingerprint {
			// Check if we have the result in store
			if existing, found := e.store.Get(incident.IncidentID); found {
				return existing, nil
			}
		}
	}

	analysisCtx := e.builder.BuildContext(incident, signals, evidences, candidates, edges)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	result, err := e.provider.Analyze(ctx, analysisCtx)
	if err != nil {
		log.Printf("[ERROR] AI Provider failed: %v", err)
		return nil, err
	}

	if err := e.validator.Validate(result, analysisCtx); err != nil {
		log.Printf("[ERROR] AI Validation failed: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrValidationFailed, err)
	}

	result.Fingerprint = fingerprint
	e.fingerprintCache[incident.IncidentID] = fingerprint
	e.store.Save(result)

	return result, nil
}

// GetAnalysis retrieves an existing analysis.
func (e *Engine) GetAnalysis(incidentID string) (*AnalysisResult, bool) {
	return e.store.Get(incidentID)
}

// CleanupExpired removes stale analyses.
func (e *Engine) CleanupExpired(now time.Time) {
	e.store.Cleanup(now)
}
