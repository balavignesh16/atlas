package aireasoning

import (
	"time"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/rca"
)

import "context"

// ReasoningProvider defines the abstraction for AI inference.
type ReasoningProvider interface {
	Analyze(ctx context.Context, input *IncidentAnalysisContext) (*AnalysisResult, error)
}

// Config holds limits and settings for the AI reasoning engine.
type Config struct {
	Enabled            bool
	Provider           string
	Endpoint           string
	Model              string
	TimeoutSeconds     int
	MaxTokens          int
	MaxEvents          int
	MaxSpans           int
	MaxServices        int
	MaxAttributes      int
	MaxStringLength    int
	RetentionSeconds   int
}

// IncidentAnalysisContext is the bounded, sanitized context passed to the AI provider.
type IncidentAnalysisContext struct {
	Incident               *incidentmodel.Incident                 `json:"incident"`
	Signals                []*incidentsignal.Signal                `json:"signals"`
	Evidence               []*evidence.Evidence                    `json:"evidence"`
	RCACandidates          []*rca.RCACandidate                     `json:"rca_candidates"`
	GraphEdges             []*correlationmodel.DependencyEdge      `json:"dependency_edges"`
}

// EvidenceReference ties an AI claim to a specific deterministic evidence ID.
type EvidenceReference struct {
	Claim       string   `json:"claim"`
	EvidenceIDs []string `json:"evidenceIds"`
}

// AnalysisResult is the strict structured output from the AI reasoning engine.
type AnalysisResult struct {
	AnalysisID                string               `json:"analysisId"`
	IncidentID                string               `json:"incidentId"`
	ExecutiveSummary          string               `json:"executiveSummary"`
	IncidentStart             time.Time            `json:"incidentStart"`
	IncidentDurationMs        int64                `json:"incidentDurationMs"`
	ObservedFacts             []EvidenceReference  `json:"observedFacts"`
	Inferences                []EvidenceReference  `json:"inferences"`
	LikelyRootCause           string               `json:"likelyRootCause"` // "AMBIGUOUS" or service name
	RootCauseConfidence       string               `json:"rootCauseConfidence"`
	AffectedServices          []string             `json:"affectedServices"`
	UnaffectedServices        []string             `json:"unaffectedServices"`
	AlternativeExplanations   []string             `json:"alternativeExplanations"`
	MissingEvidence           []string             `json:"missingEvidence"`
	RecommendedInvestigations []string             `json:"recommendedInvestigations"`
	Limitations               string               `json:"limitations"`
	GeneratedAt               time.Time            `json:"generatedAt"`
	Provider                  string               `json:"provider"`
	Model                     string               `json:"model"`
	Fingerprint               string               `json:"-"`
}
