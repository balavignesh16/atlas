package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/security"
)

type RemediationAPI struct {
	manager   *incidentmanager.Manager
	evStore   *evidence.Store
	aiEngine  *aireasoning.Engine
	rmPlanner *remediation.Planner
}

func NewRemediationAPI(manager *incidentmanager.Manager, evStore *evidence.Store, aiEngine *aireasoning.Engine, rmPlanner *remediation.Planner) *RemediationAPI {
	return &RemediationAPI{
		manager:   manager,
		evStore:   evStore,
		aiEngine:  aiEngine,
		rmPlanner: rmPlanner,
	}
}

// HandlePostPlan handles POST /api/v1/incidents/{incidentId}/remediation/plan
func (api *RemediationAPI) HandlePostPlan(w http.ResponseWriter, r *http.Request, incidentID string) {
	inc := api.manager.GetIncident(incidentID)
	if inc == nil {
		http.NotFound(w, r)
		return
	}

	evs := api.evStore.GetAll(inc.EvidenceIDs)
	evidences := make([]*evidence.Evidence, len(evs))
	for i := range evs {
		evidences[i] = &evs[i]
	}

	analysis, _ := api.aiEngine.GetAnalysis(incidentID)
	force := r.URL.Query().Get("force") == "true"

	plan, err := api.rmPlanner.GeneratePlan(r.Context(), inc, analysis, evidences, force)
	if err != nil {
		http.Error(w, "Failed to generate plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

// HandleGetPlanByIncident handles GET /api/v1/incidents/{incidentId}/remediation
func (api *RemediationAPI) HandleGetPlanByIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	plan, ok := api.rmPlanner.GetPlanByIncident(incidentID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	// Dry-run representation explicitly sets executionSupported=false
	response := struct {
		*remediation.RemediationPlan
		ExecutionSupported bool `json:"executionSupported"`
	}{
		RemediationPlan:    plan,
		ExecutionSupported: false,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleGetPlan handles GET /api/v1/remediation/{planId}
func (api *RemediationAPI) HandleGetPlan(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	planID := parts[4]

	plan, ok := api.rmPlanner.GetPlan(planID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}

// HandleApprove handles POST /api/v1/remediation/{planId}/approve
func (api *RemediationAPI) HandleApprove(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	planID := parts[4]

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	// The approval request body carries no identity field by design (see
	// docs/m29_verification_report.md): approval identity comes only from
	// the authenticated principal attached by internal/security, never from
	// anything a client could put in the request. When security is
	// disabled, no principal is present and approvedBy stays "" -- matching
	// pre-M2.9 behavior (ApprovalMetadata.ApprovedBy did not exist before).
	approvedBy := ""
	if principal, ok := security.FromContext(r.Context()); ok {
		approvedBy = principal.Name
	}

	plan, err := api.rmPlanner.ApprovePlan(planID, req.Reason, approvedBy)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := struct {
		Message string                       `json:"message"`
		Plan    *remediation.RemediationPlan `json:"plan"`
	}{
		Message: "Plan approved. Execution is not supported by this milestone.",
		Plan:    plan,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleReject handles POST /api/v1/remediation/{planId}/reject
func (api *RemediationAPI) HandleReject(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	planID := parts[4]

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	plan, err := api.rmPlanner.RejectPlan(planID, req.Reason)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plan)
}
