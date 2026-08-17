package timeline_test

import (
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/correlationmodel"
	"github.com/atlas/intelligence-engine/internal/timeline"
)

func TestBuildTimeline(t *testing.T) {
	now := time.Now()
	spans := []*correlationmodel.CorrelatedSpan{
		{SpanID: "3", StartTime: now.Add(2 * time.Second)},
		{SpanID: "1", StartTime: now},
		{SpanID: "2", StartTime: now.Add(1 * time.Second)},
	}

	tl := timeline.BuildTimeline(spans)
	if len(tl) != 3 {
		t.Fatalf("expected 3 timeline nodes, got %d", len(tl))
	}
	if tl[0].SpanID != "1" || tl[1].SpanID != "2" || tl[2].SpanID != "3" {
		t.Errorf("Timeline sorting failed")
	}
}

func TestBuildTree(t *testing.T) {
	now := time.Now()
	spans := []*correlationmodel.CorrelatedSpan{
		{SpanID: "1", ParentSpanID: "", StartTime: now},
		{SpanID: "2", ParentSpanID: "1", StartTime: now.Add(1 * time.Second)},
		{SpanID: "3", ParentSpanID: "1", StartTime: now.Add(2 * time.Second)},
		{SpanID: "4", ParentSpanID: "2", StartTime: now.Add(3 * time.Second)},
	}

	roots := timeline.BuildTree(spans)
	if len(roots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(roots))
	}
	if roots[0].SpanID != "1" {
		t.Errorf("expected root SpanID 1")
	}
	if len(roots[0].Children) != 2 {
		t.Fatalf("expected 2 children for root, got %d", len(roots[0].Children))
	}
	if len(roots[0].Children[0].Children) != 1 {
		t.Fatalf("expected 1 grandchild, got %d", len(roots[0].Children[0].Children))
	}
}
