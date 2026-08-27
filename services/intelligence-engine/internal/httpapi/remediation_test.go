package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/aireasoning/provider"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentmodel"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/remediation"
	rmprovider "github.com/atlas/intelligence-engine/internal/remediation/provider"
	"github.com/atlas/intelligence-engine/internal/security"
)

// newApprovableIncident creates a real incident (via the real
// incidentmanager.Manager, exactly as the live pipeline would) with RCA
// already set to a confidence that clears M2.6's policy gate, so a
// generated plan can be approved without needing to drive the full
// detection/RCA pipeline in this HTTP-layer test.
func newApprovableIncident(t *testing.T, mgr *incidentmanager.Manager, evStore *evidence.Store) *incidentmodel.Incident {
	t.Helper()
	ev := evidence.Evidence{
		EvidenceID:  "ev-1",
		Type:        evidence.EvidenceTypeErrorRate,
		Timestamp:   time.Now(),
		Service:     "atlas-payment-service",
		Description: "test evidence",
	}
	evStore.Add(ev)

	sig := incidentsignal.Signal{
		SignalID:  "sig-1",
		Type:      incidentsignal.SignalTypeErrorRate,
		Timestamp: time.Now(),
		Service:   "atlas-payment-service",
		Operation: "http post /api/payments",
		Value:     0.9,
		Threshold: 0.2,
		Evidence:  ev,
	}
	mgr.ProcessSignal(sig)

	incs := mgr.GetOpenIncidents()
	if len(incs) != 1 {
		t.Fatalf("expected exactly 1 open incident, got %d", len(incs))
	}
	inc := incs[0]
	inc.RCA = &incidentmodel.RootCause{Service: "atlas-payment-service", Confidence: "MEDIUM", Score: 45}
	mgr.UpdateIncident(inc)
	return mgr.GetIncident(inc.IncidentID)
}

func newTestRemediationAPI(t *testing.T) (*RemediationAPI, *incidentmanager.Manager, *evidence.Store) {
	t.Helper()
	mgr := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evidence.NewStore())
	evStore := evidence.NewStore()
	aiEngine := aireasoning.NewEngine(aireasoning.Config{Enabled: false}, provider.NewFakeProvider())
	rmPlanner := remediation.NewPlanner(remediation.Config{Enabled: false, RetentionSeconds: 3600}, rmprovider.NewFakePlanner())
	return NewRemediationAPI(mgr, evStore, aiEngine, rmPlanner), mgr, evStore
}

func generateTestPlan(t *testing.T, api *RemediationAPI, incidentID string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/remediation/plan", nil)
	rec := httptest.NewRecorder()
	api.HandlePostPlan(rec, req, incidentID)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected plan generation to succeed, got status %d body %s", rec.Code, rec.Body.String())
	}
	var plan remediation.RemediationPlan
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("failed to decode generated plan: %v", err)
	}
	return plan.PlanID
}

// 15. Authenticated principal recorded as ApprovedBy.
func TestHandleApprove_AuthenticatedPrincipalRecordedAsApprovedBy(t *testing.T) {
	api, mgr, evStore := newTestRemediationAPI(t)
	inc := newApprovableIncident(t, mgr, evStore)
	planID := generateTestPlan(t, api, inc.IncidentID)

	body := `{"reason":"restart payment"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+planID+"/approve", strings.NewReader(body))
	req = req.WithContext(security.WithPrincipal(req.Context(), security.Principal{Name: "alice", Role: security.RoleApprover}))
	rec := httptest.NewRecorder()

	api.HandleApprove(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected approval to succeed, got status %d body %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Plan *remediation.RemediationPlan `json:"plan"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Plan.Approval.ApprovedBy != "alice" {
		t.Fatalf("expected ApprovedBy=alice from the authenticated principal, got %q", resp.Plan.Approval.ApprovedBy)
	}
}

// 16 (approval half). No identity field exists in the approval request
// body at all -- confirming the body cannot express, let alone forge, an
// approver identity. The authenticated principal remains the sole source.
func TestHandleApprove_RequestBodyHasNoIdentityFieldToForge(t *testing.T) {
	api, mgr, evStore := newTestRemediationAPI(t)
	inc := newApprovableIncident(t, mgr, evStore)
	planID := generateTestPlan(t, api, inc.IncidentID)

	// A body that ATTEMPTS to smuggle an identity field; HandleApprove's
	// request struct only ever decodes "reason", so "approver"/"approvedBy"
	// here are silently ignored by json.Decode, exactly as any other
	// unrecognized field would be.
	body := `{"reason":"restart payment","approver":"admin","approvedBy":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+planID+"/approve", strings.NewReader(body))
	req = req.WithContext(security.WithPrincipal(req.Context(), security.Principal{Name: "alice", Role: security.RoleApprover}))
	rec := httptest.NewRecorder()

	api.HandleApprove(rec, req)

	var resp struct {
		Plan *remediation.RemediationPlan `json:"plan"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Plan.Approval.ApprovedBy != "alice" {
		t.Fatalf("expected the forged body fields to have zero effect and ApprovedBy to remain alice, got %q", resp.Plan.Approval.ApprovedBy)
	}
}

// Security disabled (no principal in context) -> ApprovedBy stays empty,
// matching pre-M2.9 behavior exactly (ApprovalMetadata had no such field).
func TestHandleApprove_NoPrincipalInContext_ApprovedByStaysEmpty(t *testing.T) {
	api, mgr, evStore := newTestRemediationAPI(t)
	inc := newApprovableIncident(t, mgr, evStore)
	planID := generateTestPlan(t, api, inc.IncidentID)

	body := `{"reason":"restart payment"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+planID+"/approve", strings.NewReader(body))
	rec := httptest.NewRecorder()

	api.HandleApprove(rec, req)

	var resp struct {
		Plan *remediation.RemediationPlan `json:"plan"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Plan.Approval.ApprovedBy != "" {
		t.Fatalf("expected ApprovedBy to stay empty with no authenticated principal, got %q", resp.Plan.Approval.ApprovedBy)
	}
}

// 17. Re-approval remains rejected (existing state-machine behavior,
// unaffected by the new approvedBy parameter).
func TestHandleApprove_ReApprovalRemainsRejected(t *testing.T) {
	api, mgr, evStore := newTestRemediationAPI(t)
	inc := newApprovableIncident(t, mgr, evStore)
	planID := generateTestPlan(t, api, inc.IncidentID)

	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+planID+"/approve", strings.NewReader(`{"reason":"first"}`))
	req1 = req1.WithContext(security.WithPrincipal(req1.Context(), security.Principal{Name: "alice", Role: security.RoleApprover}))
	rec1 := httptest.NewRecorder()
	api.HandleApprove(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected the first approval to succeed, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/"+planID+"/approve", strings.NewReader(`{"reason":"second"}`))
	req2 = req2.WithContext(security.WithPrincipal(req2.Context(), security.Principal{Name: "bob", Role: security.RoleApprover}))
	rec2 := httptest.NewRecorder()
	api.HandleApprove(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected re-approval to be rejected with 400, got %d", rec2.Code)
	}
}

// 18. Existing approval-state behavior (nonexistent plan) is unchanged.
func TestHandleApprove_NonexistentPlanRejected(t *testing.T) {
	api, _, _ := newTestRemediationAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/remediation/does-not-exist/approve", strings.NewReader(`{"reason":"x"}`))
	req = req.WithContext(security.WithPrincipal(req.Context(), security.Principal{Name: "alice", Role: security.RoleApprover}))
	rec := httptest.NewRecorder()

	api.HandleApprove(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for a nonexistent plan, got %d", rec.Code)
	}
}
