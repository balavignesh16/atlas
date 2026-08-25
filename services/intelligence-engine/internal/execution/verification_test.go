package execution_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
)

func newVerificationTestIncident(t *testing.T, mgr *incidentmanager.Manager, service string, lastUpdatedAt time.Time) string {
	t.Helper()
	sig := incidentsignal.Signal{
		SignalID:  "sig-" + service,
		Type:      incidentsignal.SignalTypeErrorRate,
		Timestamp: lastUpdatedAt,
		Service:   service,
		Operation: "http post /test",
		Value:     0.9,
		Threshold: 0.2,
		Evidence: evidence.Evidence{
			EvidenceID:  "ev-" + service,
			Type:        evidence.EvidenceTypeErrorRate,
			Timestamp:   lastUpdatedAt,
			Service:     service,
			Operation:   "http post /test",
			Description: "test evidence",
			Value:       0.9,
		},
	}
	mgr.ProcessSignal(sig)
	for _, inc := range mgr.GetOpenIncidents() {
		if inc.RootService == service {
			return inc.IncidentID
		}
	}
	t.Fatalf("expected an open incident to be created for %s", service)
	return ""
}

func setVerificationTestIncidentState(mgr *incidentmanager.Manager, id string, status incidentmodel.Status, lastUpdatedAt time.Time) {
	inc := mgr.GetIncident(id)
	inc.Status = status
	inc.LastUpdatedAt = lastUpdatedAt
	mgr.UpdateIncident(inc)
}

// newTestEvent builds a real, ingestion-shaped ATLASEvent -- the same kind
// buffer.EventBuffer holds from the live OTLP path -- with a genuine,
// independent Timestamp, deliberately never derived from any evaluation
// tick.
func newTestEvent(service string, ts time.Time, isError bool) event.ATLASEvent {
	status := "OK"
	if isError {
		status = "ERROR"
	}
	return event.ATLASEvent{
		EventID:       "evt-" + service + "-" + ts.String(),
		EventType:     event.EventTypeTraceSpan,
		Timestamp:     ts,
		ServiceName:   service,
		OperationName: "http post /test",
		Status:        status,
	}
}

// D. Incident already resolved -> VERIFIED, returned without waiting.
func TestVerify_AlreadyResolved_ReturnsVerifiedImmediately(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-a", time.Now())
	setVerificationTestIncidentState(mgr, id, incidentmodel.StatusResolved, time.Now())

	v := execution.NewVerifier(mgr, buffer.NewEventBuffer(100))
	start := time.Now()
	status := v.Verify(context.Background(), id, "svc-a", time.Now())
	elapsed := time.Since(start)

	if status != execution.VerificationVerified {
		t.Fatalf("expected VERIFIED, got %s", status)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected an already-resolved incident to return immediately, took %v", elapsed)
	}
}

// E. Incident disappears/unknown -> VERIFICATION_TIMEOUT, never VERIFIED.
func TestVerify_NilIncident_ReturnsTimeoutNotVerified(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evidence.NewStore())
	v := execution.NewVerifier(mgr, buffer.NewEventBuffer(100))

	status := v.Verify(context.Background(), "does-not-exist", "svc-a", time.Now())
	if status != execution.VerificationTimeout {
		t.Fatalf("expected VERIFICATION_TIMEOUT for an unknown incident, never VERIFIED from absence; got %s", status)
	}
}

// Incident resolves during verification -> VERIFIED (positive recovery
// evidence observed directly, mid-poll).
func TestVerify_ResolvesDuringPolling_ReturnsVerified(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 60 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-b", time.Now())

	go func() {
		time.Sleep(3 * time.Second)
		setVerificationTestIncidentState(mgr, id, incidentmodel.StatusResolved, time.Now())
	}()

	v := execution.NewVerifier(mgr, buffer.NewEventBuffer(100))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-b", time.Now())
	if status != execution.VerificationVerified {
		t.Fatalf("expected VERIFIED once the incident resolves mid-poll, got %s", status)
	}
}

// A. A real post-execution ERROR event for the remediated service ->
// VERIFICATION_FAILED.
func TestVerify_RealPostExecutionError_ReturnsFailed(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-c", time.Now().Add(-10*time.Second))
	executionFinishedAt := time.Now()
	buf := buffer.NewEventBuffer(100)

	go func() {
		time.Sleep(2 * time.Second)
		buf.Add(newTestEvent("svc-c", executionFinishedAt.Add(1*time.Second), true))
	}()

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-c", executionFinishedAt)
	if status != execution.VerificationFailed {
		t.Fatalf("expected VERIFICATION_FAILED given a genuine post-execution ERROR event, got %s", status)
	}
}

// B. A real post-execution event that is NOT an error must never be
// mistaken for failure evidence.
func TestVerify_PostExecutionHealthyEvent_ReturnsTimeout(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-d", time.Now().Add(-10*time.Second))
	executionFinishedAt := time.Now()
	buf := buffer.NewEventBuffer(100)
	buf.Add(newTestEvent("svc-d", executionFinishedAt.Add(1*time.Second), false))

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-d", executionFinishedAt)
	if status != execution.VerificationTimeout {
		t.Fatalf("a healthy post-execution event must never be reported as FAILED, got %s", status)
	}
}

// C. A real post-execution error on a DIFFERENT service must not affect
// this incident's verdict.
func TestVerify_PostExecutionErrorOnAnotherService_ReturnsTimeout(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-e", time.Now().Add(-10*time.Second))
	executionFinishedAt := time.Now()
	buf := buffer.NewEventBuffer(100)
	buf.Add(newTestEvent("svc-unrelated", executionFinishedAt.Add(1*time.Second), true))

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-e", executionFinishedAt)
	if status != execution.VerificationTimeout {
		t.Fatalf("an error on an unrelated service must never affect this incident's verdict, got %s", status)
	}
}

// F. THE EXACT LIVE DEFECT, reproduced directly: a real error occurred
// BEFORE execution finished, but Incident.LastUpdatedAt still advances past
// executionFinishedAt anyway (M2.4's rolling-window re-evaluation
// re-stamping "now" on every tick that still reads the stale window as
// degraded -- see docs/m274_verification_report.md). Must be TIMEOUT.
func TestVerify_StalePreExecutionErrorInBuffer_ReturnsTimeoutNotFailed(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	executionFinishedAt := time.Now()
	id := newVerificationTestIncident(t, mgr, "svc-f", executionFinishedAt.Add(-10*time.Second))
	buf := buffer.NewEventBuffer(100)
	// The only real event is BEFORE executionFinishedAt.
	buf.Add(newTestEvent("svc-f", executionFinishedAt.Add(-5*time.Second), true))
	// But LastUpdatedAt (contaminated by repeated stale-tick re-evaluation)
	// advances past executionFinishedAt anyway.
	setVerificationTestIncidentState(mgr, id, incidentmodel.StatusOpen, executionFinishedAt.Add(2*time.Second))

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-f", executionFinishedAt)
	if status != execution.VerificationTimeout {
		t.Fatalf("LastUpdatedAt advancing past executionFinishedAt must NOT independently qualify as FAILED when the only real error predates execution; got %s (this is the exact defect the live E2E exposed)", status)
	}
}

// G. Repeated stale evaluation ticks (LastUpdatedAt kept fresh every cycle,
// no real new evidence at all) must still resolve to TIMEOUT.
func TestVerify_MultipleStaleEvaluationTicks_ReturnsTimeout(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-g", time.Now())
	executionFinishedAt := time.Now()
	buf := buffer.NewEventBuffer(100)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				setVerificationTestIncidentState(mgr, id, incidentmodel.StatusOpen, time.Now())
			}
		}
	}()

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-g", executionFinishedAt)
	close(stop)
	wg.Wait()

	if status != execution.VerificationTimeout {
		t.Fatalf("expected VERIFICATION_TIMEOUT despite repeated stale-tick LastUpdatedAt refreshes with no real EventBuffer evidence, got %s", status)
	}
}

// H. A real post-execution error arriving amid surrounding stale-tick noise
// must still be detected as FAILED.
func TestVerify_RealErrorAfterMultipleStaleEvaluationTicks_ReturnsFailed(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-h", time.Now())
	executionFinishedAt := time.Now()
	buf := buffer.NewEventBuffer(100)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		count := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				count++
				setVerificationTestIncidentState(mgr, id, incidentmodel.StatusOpen, time.Now())
				if count == 2 {
					buf.Add(newTestEvent("svc-h", time.Now(), true))
				}
			}
		}
	}()

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-h", executionFinishedAt)
	close(stop)
	wg.Wait()

	if status != execution.VerificationFailed {
		t.Fatalf("expected VERIFICATION_FAILED once a real post-execution error is present, despite surrounding stale-tick noise, got %s", status)
	}
}

// I. A completely empty EventBuffer is the conservative default: TIMEOUT.
func TestVerify_EmptyEventBuffer_ReturnsTimeout(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-i", time.Now().Add(-10*time.Second))
	buf := buffer.NewEventBuffer(100)

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-i", time.Now())
	if status != execution.VerificationTimeout {
		t.Fatalf("expected VERIFICATION_TIMEOUT for a completely empty EventBuffer, got %s", status)
	}
}

// J. EventBuffer retention/eviction boundary: a genuine post-execution
// error that gets evicted by capacity pressure before Verify ever looks
// must degrade SAFELY to TIMEOUT -- never a false verdict in either
// direction, and never a crash.
func TestVerify_EventEvictedBeforePoll_SafelyReturnsTimeout(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-j", time.Now().Add(-10*time.Second))
	executionFinishedAt := time.Now()
	buf := buffer.NewEventBuffer(1) // capacity 1 makes eviction deterministic

	buf.Add(newTestEvent("svc-j", executionFinishedAt.Add(1*time.Second), true)) // genuine evidence...
	buf.Add(newTestEvent("svc-other", executionFinishedAt.Add(2*time.Second), false)) // ...evicted by this

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-j", executionFinishedAt)
	if status != execution.VerificationTimeout {
		t.Fatalf("an evicted event must degrade safely to VERIFICATION_TIMEOUT, never a false verdict; got %s", status)
	}
}

// K. Cancellation (e.g. process shutdown) must go through the same
// evidence-based final check as a plain deadline, not a hardcoded failure.
func TestVerify_ContextCancelled_UsesEvidenceBasedFinalVerdict(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 60 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-k", time.Now())
	executionFinishedAt := time.Now()

	v := execution.NewVerifier(mgr, buffer.NewEventBuffer(100))
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-k", executionFinishedAt)
	if status != execution.VerificationTimeout {
		t.Fatalf("expected VERIFICATION_TIMEOUT on cancellation with no new-failure evidence, got %s", status)
	}
}

// L. Concurrent EventBuffer writes during an in-flight Verify must be
// race-safe (run with -race) and must not affect correctness.
func TestVerify_ConcurrentEventBufferWrites_NoRace(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-l", time.Now().Add(-10*time.Second))
	executionFinishedAt := time.Now()
	buf := buffer.NewEventBuffer(1000)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				buf.Add(newTestEvent("svc-l", time.Now(), false))
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	v := execution.NewVerifier(mgr, buf)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	status := v.Verify(ctx, id, "svc-l", executionFinishedAt)
	close(stop)
	wg.Wait()

	if status != execution.VerificationTimeout {
		t.Fatalf("expected VERIFICATION_TIMEOUT (only healthy events were concurrently added), got %s", status)
	}
}

// 9. Verify must never mutate the incident it is observing.
func TestVerify_DoesNotMutateIncidentLifecycleState(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.Config{RecoverySeconds: 1 * time.Second, RetentionSeconds: time.Hour}, evidence.NewStore())
	id := newVerificationTestIncident(t, mgr, "svc-m", time.Now().Add(-10*time.Second))
	before := mgr.GetIncident(id)

	v := execution.NewVerifier(mgr, buffer.NewEventBuffer(100))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	v.Verify(ctx, id, "svc-m", time.Now())

	after := mgr.GetIncident(id)
	if after.Status != before.Status {
		t.Fatalf("Verify must never mutate incident status directly; before=%s after=%s", before.Status, after.Status)
	}
	if !after.LastUpdatedAt.Equal(before.LastUpdatedAt) {
		t.Fatalf("Verify must never mutate LastUpdatedAt; before=%v after=%v", before.LastUpdatedAt, after.LastUpdatedAt)
	}
}
