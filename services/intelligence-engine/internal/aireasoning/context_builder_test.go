package aireasoning

import (
	"strings"
	"testing"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
)

func TestContextBuilder_Sanitization(t *testing.T) {
	cfg := Config{MaxEvents: 10, MaxStringLength: 50}
	builder := NewBuilder(cfg)

	inc := &incidentmodel.Incident{
		IncidentID: "123",
		Title:      "Test password=secret token=abc",
	}

	ev := &evidence.Evidence{
		EvidenceID:  "E1",
		Description: "Failed with authorization: Bearer xyz",
	}

	ctx := builder.BuildContext(inc, nil, []*evidence.Evidence{ev}, nil, nil)

	if !strings.Contains(ctx.Incident.Title, "[REDACTED]") {
		t.Errorf("expected Title to be redacted, got %s", ctx.Incident.Title)
	}
	if !strings.Contains(ctx.Evidence[0].Description, "[REDACTED]") {
		t.Errorf("expected Evidence to be redacted, got %s", ctx.Evidence[0].Description)
	}

	// Verify the original object is NOT modified
	if strings.Contains(inc.Title, "[REDACTED]") {
		t.Errorf("original Incident was mutated")
	}
	if strings.Contains(ev.Description, "[REDACTED]") {
		t.Errorf("original Evidence was mutated")
	}
}

func TestContextBuilder_GraphValidation(t *testing.T) {
	cfg := Config{MaxEvents: 10, MaxStringLength: 50}
	builder := NewBuilder(cfg)

	edges := []*correlationmodel.DependencyEdge{
		{SourceService: "Gateway", TargetService: "Order"},
		{SourceService: "Gateway", TargetService: "Gateway"},
	}

	ctx := builder.BuildContext(&incidentmodel.Incident{}, nil, nil, nil, edges)

	if len(ctx.GraphEdges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(ctx.GraphEdges))
	}
	if ctx.GraphEdges[0].TargetService == "Gateway" {
		t.Errorf("expected self-edge to be removed")
	}
}

func TestContextBuilder_Limits(t *testing.T) {
	cfg := Config{MaxEvents: 2, MaxStringLength: 50}
	builder := NewBuilder(cfg)

	var signals []incidentsignal.Signal
	for i := 0; i < 10; i++ {
		signals = append(signals, incidentsignal.Signal{})
	}

	ctx := builder.BuildContext(&incidentmodel.Incident{}, signals, nil, nil, nil)

	if len(ctx.Signals) != 2 {
		t.Errorf("expected 2 signals due to limit, got %d", len(ctx.Signals))
	}
}
