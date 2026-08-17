package aireasoning

import (
	"log"
	"strings"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/rca"
)

// Builder constructs the bounded, sanitized context for the AI provider.
type Builder struct {
	cfg Config
}

func NewBuilder(cfg Config) *Builder {
	return &Builder{cfg: cfg}
}

func (b *Builder) BuildContext(
	incident *incidentmodel.Incident,
	signals []incidentsignal.Signal,
	evidences []*evidence.Evidence,
	candidates []*rca.RCACandidate,
	edges []*correlationmodel.DependencyEdge,
) *IncidentAnalysisContext {

	// Deep copy to prevent modifying the shared incident state
	incCopy := *incident
	b.sanitizeIncident(&incCopy)
	ctx := &IncidentAnalysisContext{
		Incident: &incCopy,
	}

	// 1. Bound and sanitize signals
	boundSignals := b.cfg.MaxEvents
	if len(signals) < boundSignals {
		boundSignals = len(signals)
	}
	ctx.Signals = make([]*incidentsignal.Signal, 0, boundSignals)
	for i := 0; i < boundSignals; i++ {
		sig := signals[i]
		b.sanitizeSignal(&sig)
		ctx.Signals = append(ctx.Signals, &sig)
	}

	// 2. Bound and sanitize evidences
	boundEv := b.cfg.MaxEvents
	if len(evidences) < boundEv {
		boundEv = len(evidences)
	}
	ctx.Evidence = make([]*evidence.Evidence, 0, boundEv)
	for i := 0; i < boundEv; i++ {
		// Deep copy to prevent modifying the store
		evCopy := *evidences[i]
		b.sanitizeEvidence(&evCopy)
		ctx.Evidence = append(ctx.Evidence, &evCopy)
	}

	// 3. Candidates
	ctx.RCACandidates = candidates

	// 4. Graph Validation
	ctx.GraphEdges = make([]*correlationmodel.DependencyEdge, 0)
	for _, edge := range edges {
		if edge.SourceService == edge.TargetService {
			log.Printf("[WARN] AI Context Builder rejected invalid dependency edge: %s -> %s", edge.SourceService, edge.TargetService)
			continue
		}
		ctx.GraphEdges = append(ctx.GraphEdges, edge)
	}

	return ctx
}

var sensitiveKeys = []string{
	"authorization", "password", "token", "secret", "api_key", "credential", "cookie",
}

// sanitizeText checks and redacts sensitive keys from text.
func (b *Builder) sanitizeText(text string) string {
	lowerText := strings.ToLower(text)
	for _, sk := range sensitiveKeys {
		if strings.Contains(lowerText, sk) {
			return "[REDACTED]"
		}
	}
	if len(text) > b.cfg.MaxStringLength {
		return text[:b.cfg.MaxStringLength] + "...[TRUNCATED]"
	}
	return text
}

// sanitize ensures no sensitive PII or credentials are leaked into the AI prompt.
func (b *Builder) sanitizeEvidence(ev *evidence.Evidence) {
	ev.Description = b.sanitizeText(ev.Description)
}

func (b *Builder) sanitizeIncident(inc *incidentmodel.Incident) {
	inc.Title = b.sanitizeText(inc.Title)
	inc.Description = b.sanitizeText(inc.Description)
}

func (b *Builder) sanitizeSignal(sig *incidentsignal.Signal) {
	sig.Evidence.Description = b.sanitizeText(sig.Evidence.Description)
}
