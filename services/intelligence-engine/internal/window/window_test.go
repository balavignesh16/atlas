package window

import (
	"testing"
	"time"
)

func TestAdd_MissingTraceIDDoesNotAffectDetectionMetrics(t *testing.T) {
	w := NewWindow()
	now := time.Now()
	w.Add(100, true, now, "")
	w.Add(200, false, now, "")

	if got := w.Count(); got != 2 {
		t.Fatalf("expected Count()=2, got %d", got)
	}
	if got := w.ErrorRate(); got != 0.5 {
		t.Fatalf("expected ErrorRate()=0.5, got %f", got)
	}
	if got := w.AverageLatency(); got != 150 {
		t.Fatalf("expected AverageLatency()=150, got %f", got)
	}
	if got := w.RecentTraceID(); got != "" {
		t.Fatalf("expected RecentTraceID()=\"\" when no observation carries one, got %q", got)
	}
}

func TestRecentTraceID_ReturnsMostRecentNonEmpty(t *testing.T) {
	w := NewWindow()
	now := time.Now()
	w.Add(100, false, now, "trace-1")
	w.Add(100, false, now.Add(1*time.Second), "")
	w.Add(100, false, now.Add(2*time.Second), "trace-3")

	if got := w.RecentTraceID(); got != "trace-3" {
		t.Fatalf("expected the most recent non-empty trace ID (trace-3), got %q", got)
	}
}

func TestRecentTraceID_SkipsBackToEarlierNonEmptyIfLatestIsEmpty(t *testing.T) {
	w := NewWindow()
	now := time.Now()
	w.Add(100, false, now, "trace-1")
	w.Add(100, false, now.Add(1*time.Second), "")

	if got := w.RecentTraceID(); got != "trace-1" {
		t.Fatalf("expected fallback to the earlier non-empty trace ID (trace-1), got %q", got)
	}
}
