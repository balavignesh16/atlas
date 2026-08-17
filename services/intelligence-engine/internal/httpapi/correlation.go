package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/timeline"
)

type CorrelationAPI struct {
	engine *correlation.Engine
}

func NewCorrelationAPI(engine *correlation.Engine) *CorrelationAPI {
	return &CorrelationAPI{engine: engine}
}

// HandleGetTrace handles GET /api/v1/correlations/traces/{traceId}
func (a *CorrelationAPI) HandleGetTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing traceId")
		return
	}

	traceID := parts[5]
	trace, exists := a.engine.GetTrace(traceID)
	if !exists {
		writeError(w, http.StatusNotFound, "TRACE_NOT_FOUND", "Trace not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trace)
}

// HandleGetTraceTree handles GET /api/v1/correlations/traces/{traceId}/tree
func (a *CorrelationAPI) HandleGetTraceTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 7 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing traceId")
		return
	}

	traceID := parts[5]
	trace, exists := a.engine.GetTrace(traceID)
	if !exists {
		writeError(w, http.StatusNotFound, "TRACE_NOT_FOUND", "Trace not found")
		return
	}

	tree := timeline.BuildTree(trace.Spans)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tree)
}

// HandleGetTraceTimeline handles GET /api/v1/correlations/traces/{traceId}/timeline
func (a *CorrelationAPI) HandleGetTraceTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 7 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing traceId")
		return
	}

	traceID := parts[5]
	trace, exists := a.engine.GetTrace(traceID)
	if !exists {
		writeError(w, http.StatusNotFound, "TRACE_NOT_FOUND", "Trace not found")
		return
	}

	tl := timeline.BuildTimeline(trace.Spans)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tl)
}

func writeError(w http.ResponseWriter, status int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":   errorCode,
		"message": message,
	})
}
