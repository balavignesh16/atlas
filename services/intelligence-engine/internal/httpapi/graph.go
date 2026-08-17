package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atlas/intelligence-engine/internal/graph"
)

type GraphAPI struct {
	graph *graph.DependencyGraph
}

func NewGraphAPI(g *graph.DependencyGraph) *GraphAPI {
	return &GraphAPI{graph: g}
}

// HandleGetGraph handles GET /api/v1/graph
func (a *GraphAPI) HandleGetGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	snapshot := a.graph.GetSnapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snapshot)
}

// HandleGetServiceDependencies handles GET /api/v1/graph/services/{serviceName}
func (a *GraphAPI) HandleGetServiceDependencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 6 {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing serviceName")
		return
	}

	serviceName := parts[5]
	incoming, outgoing := a.graph.GetServiceDependencies(serviceName)

	if len(incoming) == 0 && len(outgoing) == 0 {
		writeError(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "Service not found or no dependencies observed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service":  serviceName,
		"incoming": incoming,
		"outgoing": outgoing,
	})
}

// HandleGetEdges handles GET /api/v1/graph/edges
func (a *GraphAPI) HandleGetEdges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	edges := a.graph.GetEdges()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(edges)
}
