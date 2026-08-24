package incidentmanager

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
)

func newTestSignal(traceID string, t time.Time) incidentsignal.Signal {
	return incidentsignal.Signal{
		SignalID:  "sig-" + traceID,
		Type:      incidentsignal.SignalTypeErrorRate,
		Timestamp: t,
		Service:   "atlas-payment-service",
		Operation: "http post /api/payments",
		Value:     0.5,
		Threshold: 0.2,
		TraceID:   traceID,
		Evidence: evidence.Evidence{
			EvidenceID:  "ev-" + traceID,
			Type:        evidence.EvidenceTypeErrorRate,
			Timestamp:   t,
			Service:     "atlas-payment-service",
			Description: "test",
		},
	}
}

func TestProcessSignal_DuplicateTraceIDsAreIgnored(t *testing.T) {
	m := NewManager(DefaultConfig(), evidence.NewStore())
	now := time.Now()

	m.ProcessSignal(newTestSignal("trace-1", now))
	m.ProcessSignal(newTestSignal("trace-1", now.Add(1*time.Second)))

	incs := m.GetOpenIncidents()
	if len(incs) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(incs))
	}
	if got := len(incs[0].TraceIDs); got != 1 {
		t.Fatalf("expected exactly 1 distinct trace ID after a duplicate, got %d (%v)", got, incs[0].TraceIDs)
	}
}

func TestProcessSignal_TraceIDsAreBoundedWithOldestEvictedFirst(t *testing.T) {
	m := NewManager(DefaultConfig(), evidence.NewStore())
	now := time.Now()

	total := MaxTraceIDsPerIncident + 5
	for i := 0; i < total; i++ {
		m.ProcessSignal(newTestSignal(fmt.Sprintf("trace-%d", i), now.Add(time.Duration(i)*time.Second)))
	}

	incs := m.GetOpenIncidents()
	if len(incs) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(incs))
	}
	traceIDs := incs[0].TraceIDs
	if len(traceIDs) != MaxTraceIDsPerIncident {
		t.Fatalf("expected TraceIDs capped at %d, got %d (%v)", MaxTraceIDsPerIncident, len(traceIDs), traceIDs)
	}

	// The oldest entries (trace-0 .. trace-4) must have been evicted first;
	// the most recent MaxTraceIDsPerIncident must remain, in order.
	for i, tid := range traceIDs {
		want := fmt.Sprintf("trace-%d", i+5)
		if tid != want {
			t.Fatalf("expected FIFO eviction to leave %q at position %d, got %q (%v)", want, i, tid, traceIDs)
		}
	}
}

func TestProcessSignal_MissingTraceIDNeverBreaksIncidentCreation(t *testing.T) {
	m := NewManager(DefaultConfig(), evidence.NewStore())
	m.ProcessSignal(newTestSignal("", time.Now()))

	incs := m.GetOpenIncidents()
	if len(incs) != 1 {
		t.Fatalf("expected an incident to still be created with an empty TraceID, got %d incidents", len(incs))
	}
	if len(incs[0].TraceIDs) != 0 {
		t.Fatalf("expected no TraceIDs recorded for an empty trace ID, got %v", incs[0].TraceIDs)
	}
}

// Concurrent ProcessSignal calls (same fingerprint, distinct trace IDs) must
// not race on Incident.TraceIDs. Run with -race.
func TestProcessSignal_ConcurrentTraceIDAppendsAreThreadSafe(t *testing.T) {
	m := NewManager(DefaultConfig(), evidence.NewStore())
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.ProcessSignal(newTestSignal(fmt.Sprintf("trace-%d", i), now))
		}(i)
	}
	wg.Wait()

	incs := m.GetOpenIncidents()
	if len(incs) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(incs))
	}
	if got := len(incs[0].TraceIDs); got != MaxTraceIDsPerIncident {
		t.Fatalf("expected TraceIDs capped at %d after concurrent appends, got %d", MaxTraceIDsPerIncident, got)
	}
}
