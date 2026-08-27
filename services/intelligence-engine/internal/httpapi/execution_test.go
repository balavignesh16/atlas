package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/execution"
	execprovider "github.com/atlas/intelligence-engine/internal/execution/provider"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/remediation"
	rmprovider "github.com/atlas/intelligence-engine/internal/remediation/provider"
	"github.com/atlas/intelligence-engine/internal/security"
)

type fakeVerifier struct{ status execution.VerificationStatus }

func (f *fakeVerifier) Verify(ctx context.Context, incidentID, serviceName string, executionFinishedAt time.Time) execution.VerificationStatus {
	return f.status
}

// newApprovedTestPlan drives a real incident through the real Planner
// (generate -> approve, bypassing HTTP) to produce a genuinely APPROVED
// plan with a valid fingerprint, exactly what Guard.Check requires --
// mirrors the exact setup used by execution/engine_test.go and
// docker_integration_test.go elsewhere in this codebase.
func newApprovedTestPlan(t *testing.T) (*remediation.Planner, *remediation.RemediationPlan) {
	t.Helper()
	mgr := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evidence.NewStore())
	evStore := evidence.NewStore()

	ev := evidence.Evidence{EvidenceID: "ev-1", Type: evidence.EvidenceTypeErrorRate, Timestamp: time.Now(), Service: "atlas-payment-service", Description: "test evidence"}
	evStore.Add(ev)

	mgr.ProcessSignal(incidentsignal.Signal{
		SignalID: "sig-1", Type: incidentsignal.SignalTypeErrorRate, Timestamp: time.Now(),
		Service: "atlas-payment-service", Operation: "http post /api/payments", Value: 0.9, Threshold: 0.2, Evidence: ev,
	})
	incs := mgr.GetOpenIncidents()
	if len(incs) != 1 {
		t.Fatalf("expected exactly 1 open incident, got %d", len(incs))
	}
	inc := incs[0]
	inc.RCA = &incidentmodel.RootCause{Service: "atlas-payment-service", Confidence: "MEDIUM", Score: 45}
	mgr.UpdateIncident(inc)
	inc = mgr.GetIncident(inc.IncidentID)

	rmPlanner := remediation.NewPlanner(remediation.Config{Enabled: false, RetentionSeconds: 3600}, rmprovider.NewFakePlanner())
	plan, err := rmPlanner.GeneratePlan(context.Background(), inc, nil, []*evidence.Evidence{&ev}, false)
	if err != nil {
		t.Fatalf("unexpected plan generation error: %v", err)
	}
	approved, err := rmPlanner.ApprovePlan(plan.PlanID, "test setup", "setup-approver")
	if err != nil {
		t.Fatalf("unexpected approval error: %v", err)
	}
	return rmPlanner, approved
}

func newTestExecutionAPI(t *testing.T, rmPlanner *remediation.Planner) *ExecutionAPI {
	t.Helper()
	guard := execution.NewGuard(true)
	store := execution.NewStore(3600)
	fakeExec := execprovider.NewFakeExecutor()
	verifier := &fakeVerifier{status: execution.VerificationVerified}
	engine := execution.NewEngine(guard, fakeExec, verifier, store, 5)
	return NewExecutionAPI(engine, rmPlanner)
}

// 19. Authenticated executor identity recorded.
func TestHandleExecute_AuthenticatedPrincipalRecordedAsApprover(t *testing.T) {
	rmPlanner, plan := newApprovedTestPlan(t)
	api := newTestExecutionAPI(t, rmPlanner)

	body := `{"actionId":"` + plan.Actions[0].ActionID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+plan.PlanID+"/execute", strings.NewReader(body))
	req = req.WithContext(security.WithPrincipal(req.Context(), security.Principal{Name: "carol", Role: security.RoleExecutor}))
	rec := httptest.NewRecorder()

	api.HandleExecute(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected execution to succeed, got status %d body %s", rec.Code, rec.Body.String())
	}
	var record execution.ExecutionRecord
	if err := json.NewDecoder(rec.Body).Decode(&record); err != nil {
		t.Fatalf("failed to decode execution record: %v", err)
	}
	if record.Approver != "carol" {
		t.Fatalf("expected the executor identity to be the authenticated principal carol, got %q", record.Approver)
	}
}

// 20 / mandatory forged-identity test (section 12): authenticated identity
// is carol; the request body claims approver=admin. The trusted executor
// identity recorded MUST be carol, never admin.
func TestHandleExecute_ForgedApproverInBody_CannotOverrideAuthenticatedIdentity(t *testing.T) {
	rmPlanner, plan := newApprovedTestPlan(t)
	api := newTestExecutionAPI(t, rmPlanner)

	body := `{"actionId":"` + plan.Actions[0].ActionID + `","approver":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+plan.PlanID+"/execute", strings.NewReader(body))
	req = req.WithContext(security.WithPrincipal(req.Context(), security.Principal{Name: "carol", Role: security.RoleExecutor}))
	rec := httptest.NewRecorder()

	api.HandleExecute(rec, req)

	var record execution.ExecutionRecord
	if err := json.NewDecoder(rec.Body).Decode(&record); err != nil {
		t.Fatalf("failed to decode execution record: %v", err)
	}
	if record.Approver != "carol" {
		t.Fatalf("SECURITY: forged body approver=admin must NOT override the authenticated identity; expected carol, got %q", record.Approver)
	}
	if record.Approver == "admin" {
		t.Fatal("SECURITY: the forged identity 'admin' was trusted -- this is exactly the vulnerability M2.9 exists to close")
	}
}

// Security disabled (no principal in context): falls back to the
// client-supplied req.Approver exactly as pre-M2.9 behavior did.
func TestHandleExecute_NoPrincipalInContext_FallsBackToBodyApprover(t *testing.T) {
	rmPlanner, plan := newApprovedTestPlan(t)
	api := newTestExecutionAPI(t, rmPlanner)

	body := `{"actionId":"` + plan.Actions[0].ActionID + `","approver":"legacy-caller"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+plan.PlanID+"/execute", strings.NewReader(body))
	rec := httptest.NewRecorder()

	api.HandleExecute(rec, req)

	var record execution.ExecutionRecord
	if err := json.NewDecoder(rec.Body).Decode(&record); err != nil {
		t.Fatalf("failed to decode execution record: %v", err)
	}
	if record.Approver != "legacy-caller" {
		t.Fatalf("expected pre-M2.9 fallback behavior (body approver used) when unauthenticated, got %q", record.Approver)
	}
}

// 23. Existing guard behavior remains intact: an unapproved plan is still
// rejected, unaffected by the identity-sourcing change.
func TestHandleExecute_GuardStillRejectsUnapprovedPlan(t *testing.T) {
	mgr := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evidence.NewStore())
	evStore := evidence.NewStore()
	ev := evidence.Evidence{EvidenceID: "ev-1", Type: evidence.EvidenceTypeErrorRate, Timestamp: time.Now(), Service: "atlas-payment-service", Description: "test evidence"}
	evStore.Add(ev)
	mgr.ProcessSignal(incidentsignal.Signal{
		SignalID: "sig-1", Type: incidentsignal.SignalTypeErrorRate, Timestamp: time.Now(),
		Service: "atlas-payment-service", Operation: "http post /api/payments", Value: 0.9, Threshold: 0.2, Evidence: ev,
	})
	inc := mgr.GetOpenIncidents()[0]
	inc.RCA = &incidentmodel.RootCause{Service: "atlas-payment-service", Confidence: "MEDIUM", Score: 45}
	mgr.UpdateIncident(inc)
	inc = mgr.GetIncident(inc.IncidentID)

	rmPlanner := remediation.NewPlanner(remediation.Config{Enabled: false, RetentionSeconds: 3600}, rmprovider.NewFakePlanner())
	plan, err := rmPlanner.GeneratePlan(context.Background(), inc, nil, []*evidence.Evidence{&ev}, false)
	if err != nil {
		t.Fatalf("unexpected plan generation error: %v", err)
	}
	// Deliberately NOT approved.

	api := newTestExecutionAPI(t, rmPlanner)
	body := `{"actionId":"` + plan.Actions[0].ActionID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+plan.PlanID+"/execute", strings.NewReader(body))
	req = req.WithContext(security.WithPrincipal(req.Context(), security.Principal{Name: "carol", Role: security.RoleExecutor}))
	rec := httptest.NewRecorder()

	api.HandleExecute(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected Guard to reject an unapproved plan with 403, got %d", rec.Code)
	}
}
