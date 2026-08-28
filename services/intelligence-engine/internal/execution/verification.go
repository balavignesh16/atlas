package execution

import (
	"context"
	"log"
	"time"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/event"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
)

// verificationGrace is added on top of an incident's own remaining recovery
// time to absorb normal scheduling/evaluation-loop jitter. It replaces the
// old unconditional 5s pre-sleep: previously that sleep was spent no matter
// how much of the incident's recovery window was already elapsed, which is
// exactly what made the fixed ~35s budget race against M2.4's independent
// ~30s recovery clock (see docs/m273_verification_report.md's Run 3
// forensic finding). Now it's a fixed top-up on a dynamically computed wait.
//
// This budget is intentionally left as-is by the M2.7.4 FAILED/TIMEOUT
// correctness fix: it can only affect how soon VERIFICATION_TIMEOUT is
// reached, never whether a verdict is wrong -- VERIFIED still depends only
// on Incident.Status, and FAILED no longer depends on Incident.LastUpdatedAt
// at all (see finalVerdict). A longer wait is a quality knob, not a
// correctness requirement, so it isn't touched here.
const verificationGrace = 5 * time.Second

type Verifier struct {
	manager     *incidentmanager.Manager
	eventBuffer *buffer.EventBuffer
}

func NewVerifier(manager *incidentmanager.Manager, eventBuffer *buffer.EventBuffer) *Verifier {
	return &Verifier{manager: manager, eventBuffer: eventBuffer}
}

// Verify implements post-execution verification using the M2.4 source of
// truth for recovery (Incident.Status) and the real event-ingestion path
// for failure evidence (buffer.EventBuffer). It never infers VERIFIED from
// the incident's absence, and it only reports VerificationFailed when a
// genuinely observed ERROR event for this service has a real Timestamp
// strictly after executionFinishedAt -- never from Incident.LastUpdatedAt,
// Evidence.Timestamp, or any other evaluation-tick-stamped field, all of
// which get re-stamped to "now" on every M2.4 evaluation cycle regardless
// of whether the underlying window contents are fresh or stale (see
// docs/m274_verification_report.md).
//
// M2.13: Status=="RESOLVED" is NOT sufficient on its own for VERIFIED (see
// verdictForResolved). The incident manager's recovery clock resolves an
// incident purely from absence of a fresh signal touching THAT SPECIFIC
// incident record -- which can happen even while a genuinely new
// post-execution failure already sits in the EventBuffer, if the fresh
// failure's signal ends up creating a different incident record instead of
// refreshing this one (see docs/m213_verification_report.md). Every path
// that observes RESOLVED now checks for genuine post-execution failure
// evidence first, exactly like the existing FAILED path already does.
func (v *Verifier) Verify(ctx context.Context, incidentID string, serviceName string, executionFinishedAt time.Time) VerificationStatus {
	log.Printf("[INFO] Verifying execution outcome for incident %s, service %s", incidentID, serviceName)

	inc := v.manager.GetIncident(incidentID)
	if inc == nil {
		// No defensible signal either way: this incident ID was never seen,
		// or has already aged out of retention. Absence is not recovery.
		return VerificationTimeout
	}
	if inc.Status == "RESOLVED" {
		return v.verdictForResolved(serviceName, executionFinishedAt)
	}

	// Compute how much of the incident's own recovery clock is left, so the
	// wait tracks the actual thing we're waiting on instead of a flat
	// budget that may be too short (still recovering) or needlessly long
	// (already almost recovered).
	recoverySeconds := v.manager.RecoverySeconds()
	remaining := recoverySeconds - time.Since(inc.LastUpdatedAt)
	if remaining < 0 {
		remaining = 0
	}
	budget := remaining + verificationGrace

	deadline := time.NewTimer(budget)
	defer deadline.Stop()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[WARN] Verification cancelled for incident %s", incidentID)
			return v.finalVerdict(incidentID, serviceName, executionFinishedAt)
		case <-deadline.C:
			log.Printf("[INFO] Verification deadline reached for incident %s", incidentID)
			return v.finalVerdict(incidentID, serviceName, executionFinishedAt)
		case <-ticker.C:
			latest := v.manager.GetIncident(incidentID)
			if latest == nil {
				continue
			}
			if latest.Status == "RESOLVED" {
				return v.verdictForResolved(serviceName, executionFinishedAt)
			}
			// Still open: keep waiting. Whether it counts as FAILED or
			// TIMEOUT once the deadline arrives is decided by finalVerdict,
			// using real EventBuffer evidence, not this incident's own
			// (evaluation-tick-contaminated) LastUpdatedAt.
		}
	}
}

// finalVerdict is the single place that turns "we stopped waiting" into a
// terminal status. VERIFIED requires a direct Status=="RESOLVED" observation
// AND the absence of genuine post-execution failure evidence (see
// verdictForResolved). FAILED requires positive evidence from the real
// EventBuffer: an actual ingested ERROR event for this service, timestamped
// strictly after executionFinishedAt. Absence of either -- including a
// stale error still sitting inside M2.4's rolling window, which keeps
// re-affirming itself on every evaluation tick without any new real event
// -- conservatively resolves to TIMEOUT, never FAILED.
func (v *Verifier) finalVerdict(incidentID string, serviceName string, executionFinishedAt time.Time) VerificationStatus {
	final := v.manager.GetIncident(incidentID)
	if final != nil && final.Status == "RESOLVED" {
		return v.verdictForResolved(serviceName, executionFinishedAt)
	}
	if v.hasGenuinePostExecutionFailure(serviceName, executionFinishedAt) {
		return VerificationFailed
	}
	return VerificationTimeout
}

// verdictForResolved decides between VERIFIED and FAILED once an incident's
// Status has been observed as RESOLVED. RESOLVED alone is not sufficient
// (M2.13): the incident manager's recovery clock can resolve an incident
// purely from absence of a fresh signal touching THAT SPECIFIC incident
// record, independent of whether the remediated service is actually
// healthy -- a genuinely new post-execution failure can already be sitting
// in the EventBuffer at the exact moment RESOLVED is observed, e.g. because
// the fresh failure's signal created a new incident record instead of
// refreshing this one. Genuine EventBuffer evidence always takes precedence
// over an incidental RESOLVED observation, using the exact same evidence
// check the FAILED path has always used -- no new evidence mechanism.
func (v *Verifier) verdictForResolved(serviceName string, executionFinishedAt time.Time) VerificationStatus {
	if v.hasGenuinePostExecutionFailure(serviceName, executionFinishedAt) {
		return VerificationFailed
	}
	return VerificationVerified
}

// hasGenuinePostExecutionFailure scans the real, already-populated event
// ingestion buffer for an actual ERROR event belonging to serviceName whose
// genuine (OTel-span-derived) Timestamp is strictly after
// executionFinishedAt. This is deliberately independent of M2.4's
// Signal/Evidence/Incident layer: those are all re-stamped with the
// evaluation tick's wall-clock time on every cycle a rolling window still
// reads above threshold, which does not distinguish a genuinely new failure
// from repeated re-evaluation of old, already-observed data. Event.Timestamp
// does not have that problem -- it is set once, at ingestion, from the real
// span.
func (v *Verifier) hasGenuinePostExecutionFailure(serviceName string, executionFinishedAt time.Time) bool {
	for _, e := range v.eventBuffer.GetAll() {
		if e.EventType != event.EventTypeTraceSpan {
			continue
		}
		if e.ServiceName != serviceName {
			continue
		}
		if !e.Timestamp.After(executionFinishedAt) {
			continue
		}
		if event.IsErrorStatus(e.Status, e.Attributes) {
			return true
		}
	}
	return false
}
