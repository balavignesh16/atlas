package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/atlas/intelligence-engine/internal/remediation"
	"github.com/atlas/intelligence-engine/internal/security"
)

type ExecutionAPI struct {
	engine  *execution.Engine
	planner *remediation.Planner
}

func NewExecutionAPI(engine *execution.Engine, planner *remediation.Planner) *ExecutionAPI {
	return &ExecutionAPI{
		engine:  engine,
		planner: planner,
	}
}

// HandleExecute handles POST /api/v1/remediation/{planId}/execute
func (api *ExecutionAPI) HandleExecute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	planID := parts[4]

	plan, ok := api.planner.GetPlan(planID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var req struct {
		ActionID string `json:"actionId"`
		// Approver is retained for backward compatibility (M2.7/M2.8 test
		// scripts still send it, and it's used as a fallback display value
		// when security is disabled) but is NEVER the trust source for
		// executor identity when a request is authenticated -- see below.
		Approver string `json:"approver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// Executor identity comes from the authenticated principal (M2.9) when
	// one is present, never from the client-supplied req.Approver -- a
	// caller cannot claim to be someone else by setting that field. Falls
	// back to req.Approver only when no principal is attached, i.e.
	// security is disabled (ATLAS_SECURITY_ENABLED=false, the default),
	// preserving pre-M2.9 behavior exactly.
	approver := req.Approver
	if principal, ok := security.FromContext(r.Context()); ok {
		approver = principal.Name
	}

	record, err := api.engine.ExecutePlanAction(r.Context(), plan, req.ActionID, approver)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}

// HandleGetExecution handles GET /api/v1/executions/{executionId}
func (api *ExecutionAPI) HandleGetExecution(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	execID := parts[4]

	record, ok := api.engine.GetRecord(execID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}

// HandleGetExecutionsByIncident handles GET /api/v1/incidents/{incidentId}/executions
func (api *ExecutionAPI) HandleGetExecutionsByIncident(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	incidentID := parts[4]

	records := api.engine.GetRecordsByIncident(incidentID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}
