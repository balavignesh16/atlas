//go:build !windows

package docker

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/remediation/action"
)

// stubVerifier stands in for M2.4's incident manager, which this test does
// not wire up -- the incident-resolution path is already covered by M2.4's
// own tests and by the incidentmanager correlation tests. This test's only
// job is to prove the execution layer itself: a real approved plan, run
// through the real execution.Engine and the real Docker adapter, actually
// restarts a real container.
type stubVerifier struct{}

func (stubVerifier) Verify(ctx context.Context, incidentID string, serviceName string, executionFinishedAt time.Time) execution.VerificationStatus {
	return execution.VerificationVerified
}

// TestDockerAdapter_RealRestartAgainstLiveContainer proves the production
// path -- execution.Engine driving the real Docker SDK adapter -- genuinely
// restarts a real container, using a plan approved directly in Go rather
// than through the HTTP planner. That's a deliberate choice: a real,
// evidence-driven cascade cannot reliably clear M2.6's confidence/ambiguity
// safety gates under the current (unmodified, out-of-scope for M2.7.1)
// rca.Engine scoring formula -- a single evidence type maxes out at a score
// well under the MEDIUM-confidence threshold, and correlation deliberately
// does not merge same-service, different-signal-type incidents (that's
// intra-service fragmentation, not the cross-service cascade problem this
// milestone solves). See docs/roadmap-checklist.md for the full trace. That
// gate is a real, separate finding for a future RCA-scoring milestone -- it
// is not evidence that execution itself is broken, and this test exists to
// prove exactly that distinction against real infrastructure.
//
// Not part of the normal `go test ./...` / CI run: requires a live Docker
// daemon reachable at the default socket AND a running
// `atlas-payment-service-1` container (i.e. `docker compose up`).
//
// Run with:
//   ATLAS_DOCKER_INTEGRATION_TEST=true go test ./internal/infrastructure/docker/... -run TestDockerAdapter_RealRestartAgainstLiveContainer -v
func TestDockerAdapter_RealRestartAgainstLiveContainer(t *testing.T) {
	if os.Getenv("ATLAS_DOCKER_INTEGRATION_TEST") != "true" {
		t.Skip("skipping live-Docker integration test; set ATLAS_DOCKER_INTEGRATION_TEST=true to run (requires `docker compose up` with atlas-payment-service-1 running)")
	}

	adapter, err := NewAdapter()
	if err != nil {
		t.Fatalf("failed to construct the real Docker adapter (is the Docker socket mounted/reachable?): %v", err)
	}

	guard := execution.NewGuard(true)
	store := execution.NewStore(3600)
	engine := execution.NewEngine(guard, adapter, stubVerifier{}, store, 30)

	const fingerprint = "integration-test-fingerprint"
	plan := &remediation.RemediationPlan{
		PlanID:      "integration-test-plan",
		IncidentID:  "integration-test-incident",
		Status:      remediation.StatusApproved,
		Fingerprint: fingerprint,
		Approval: remediation.ApprovalMetadata{
			ApprovedFingerprint: fingerprint,
		},
		Actions: []action.RemediationAction{
			{
				ActionID:      "integration-test-action",
				Type:          action.TypeRestartService,
				TargetService: "atlas-payment-service",
				EvidenceIDs:   []string{"integration-test-evidence"},
			},
		},
	}

	record, err := engine.ExecutePlanAction(context.Background(), plan, "integration-test-action", "integration-test")
	if err != nil {
		t.Fatalf("ExecutePlanAction failed: %v", err)
	}

	if record.ExecutionStatus != execution.StatusExecuted {
		t.Fatalf("expected the real Docker adapter to successfully restart atlas-payment-service-1, got status=%s message=%q error=%q",
			record.ExecutionStatus, record.Message, record.Error)
	}
	t.Logf("Real Docker restart succeeded: %s", record.Message)

	// Idempotency: a second call with the same planId/actionId must return
	// the cached record rather than restarting the container again.
	record2, err := engine.ExecutePlanAction(context.Background(), plan, "integration-test-action", "integration-test")
	if err != nil {
		t.Fatalf("second ExecutePlanAction call failed: %v", err)
	}
	if record2.ExecutionID != record.ExecutionID {
		t.Fatalf("expected idempotent replay to return the same executionId, got a new one (%s vs %s)", record2.ExecutionID, record.ExecutionID)
	}
}
