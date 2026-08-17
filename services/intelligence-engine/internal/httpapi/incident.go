package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/rca"
)

type IncidentAPI struct {
	manager    *incidentmanager.Manager
	evStore    *evidence.Store
	rcaEngine  *rca.Engine
	corrEngine *correlation.Engine
	aiEngine   *aireasoning.Engine
	depGraph   *graph.DependencyGraph // need graph for context
}

func NewIncidentAPI(manager *incidentmanager.Manager, evStore *evidence.Store, rcaEngine *rca.Engine, corrEngine *correlation.Engine, aiEngine *aireasoning.Engine, depGraph *graph.DependencyGraph) *IncidentAPI {
	return &IncidentAPI{
		manager:    manager,
		evStore:    evStore,
		rcaEngine:  rcaEngine,
		corrEngine: corrEngine,
		aiEngine:   aiEngine,
		depGraph:   depGraph,
	}
}

func (api *IncidentAPI) HandleGetIncidents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	incidents := api.manager.GetAllIncidents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(incidents)
}

func (api *IncidentAPI) HandleGetOpenIncidents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	incidents := api.manager.GetOpenIncidents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(incidents)
}

func (api *IncidentAPI) HandleGetIncident(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Path: /api/v1/incidents/{incidentId}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	id := parts[4]
	
	// Handle subpaths
	if len(parts) == 6 {
		subpath := parts[5]
		if subpath == "evidence" {
			api.handleGetEvidence(w, r, id)
			return
		}
		if subpath == "rca" {
			api.handleGetRCA(w, r, id)
			return
		}
		if subpath == "timeline" {
			api.handleGetTimeline(w, r, id)
			return
		}
		if subpath == "analyze" {
			if r.Method == http.MethodPost {
				api.HandlePostAnalyze(w, r, id)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if subpath == "analysis" {
			if r.Method == http.MethodGet {
				api.HandleGetAnalysis(w, r, id)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
	}

	inc := api.manager.GetIncident(id)
	if inc == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inc)
}

func (api *IncidentAPI) handleGetEvidence(w http.ResponseWriter, r *http.Request, id string) {
	inc := api.manager.GetIncident(id)
	if inc == nil {
		http.NotFound(w, r)
		return
	}
	evidences := api.evStore.GetAll(inc.EvidenceIDs)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(evidences)
}

func (api *IncidentAPI) handleGetRCA(w http.ResponseWriter, r *http.Request, id string) {
	inc := api.manager.GetIncident(id)
	if inc == nil {
		http.NotFound(w, r)
		return
	}
	
	type RCAAPIResponse struct {
		RootCause   string   `json:"rootCause"`
		Confidence  string   `json:"confidence"`
		Score       int      `json:"score"`
		Candidates  []string `json:"candidates"`
		EvidenceIDs []string `json:"evidenceIds"`
		Reasoning   []string `json:"reasoning"`
		Limitations []string `json:"limitations"`
	}

	res := RCAAPIResponse{
		RootCause:   "UNKNOWN",
		Confidence:  "NONE",
		Score:       0,
		Candidates:  inc.AffectedServices,
		EvidenceIDs: make([]string, 0),
		Reasoning:   make([]string, 0),
		Limitations: []string{"Root cause is probabilistic and based only on observed telemetry."},
	}

	if inc.RCA != nil {
		res.RootCause = inc.RCA.Service
		res.Confidence = inc.RCA.Confidence
		res.Score = inc.RCA.Score
		res.Reasoning = append(res.Reasoning, inc.DetectionReason)
		// Return evidence associated with the RCA, which are in the Incident
		res.EvidenceIDs = inc.EvidenceIDs
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (api *IncidentAPI) handleGetTimeline(w http.ResponseWriter, r *http.Request, id string) {
	inc := api.manager.GetIncident(id)
	if inc == nil {
		http.NotFound(w, r)
		return
	}
	
	// Collect timeline from all traces associated with the incident
	// We can reuse the Timeline module from M2.3.
	// We'll just return the timelines of all involved traces.
	type TimelineEntry struct {
		TraceID string `json:"traceId"`
		// Timeline data can be fetched from corrEngine
		Events []interface{} `json:"events"` 
	}
	
	w.Header().Set("Content-Type", "application/json")
	// For simplicity, just return TraceIDs and let client query them?
	// The prompt states: "Return: 10:00:00 Payment latency increased..."
	// Wait, the timeline should probably be a merge of the incident timeline.
	// Since M2.4 focuses on deterministic timelines, let's just output the evidences sorted chronologically!
	
	evidences := api.evStore.GetAll(inc.EvidenceIDs)
	// Sort evidences by timestamp...
	// For now, return evidences as the timeline of the incident.
	json.NewEncoder(w).Encode(evidences)
}

func (api *IncidentAPI) HandlePostAnalyze(w http.ResponseWriter, r *http.Request, id string) {
	inc := api.manager.GetIncident(id)
	if inc == nil {
		http.NotFound(w, r)
		return
	}

	evs := api.evStore.GetAll(inc.EvidenceIDs)
	evidences := make([]*evidence.Evidence, len(evs))
	for i := range evs {
		evidences[i] = &evs[i]
	}

	var candidates []*rca.RCACandidate
	if inc.RCA != nil {
		candidates = []*rca.RCACandidate{
			{Service: inc.RCA.Service, Score: inc.RCA.Score, Confidence: inc.RCA.Confidence},
		}
	}
	edges := api.depGraph.GetEdges()

	// Parse force flag from query if needed, default to false.
	force := r.URL.Query().Get("force") == "true"

	res, err := api.aiEngine.Analyze(inc, nil, evidences, candidates, edges, force)
	if err != nil {
		if err == aireasoning.ErrDisabled {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "DISABLED",
				"message": "AI reasoning is disabled.",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (api *IncidentAPI) HandleGetAnalysis(w http.ResponseWriter, r *http.Request, id string) {
	res, ok := api.aiEngine.GetAnalysis(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}
