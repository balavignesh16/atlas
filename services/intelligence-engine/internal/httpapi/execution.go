package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atlas/intelligence-engine/internal/execution"
	"github.com/atlas/intelligence-engine/internal/remediation"
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
		Approver string `json:"approver"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	record, err := api.engine.ExecutePlanAction(r.Context(), plan, req.ActionID, req.Approver)
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
