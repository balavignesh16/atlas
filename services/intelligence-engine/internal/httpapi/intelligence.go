package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/atlas/intelligence-engine/internal/serviceintel"
)

// IntelligenceAPI exposes the Phase 7D composed, read-only per-service view.
// Like RegistryAPI, there are no mutation endpoints here -- the Assembler it
// wraps only ever reads from internal/registry, internal/graph, and
// internal/incidentmanager, never writes to any of them.
type IntelligenceAPI struct {
	assembler *serviceintel.Assembler
}

func NewIntelligenceAPI(assembler *serviceintel.Assembler) *IntelligenceAPI {
	return &IntelligenceAPI{assembler: assembler}
}

// registryView mirrors serviceintel.RegistryView. Status/Provenance/
// Confidence and both timestamps are omitted entirely when Known is false --
// a caller must never be able to mistake "the registry never heard of this
// service" for "the registry knows about it and every field happens to be
// zero."
type registryView struct {
	Known           bool       `json:"known"`
	Status          string     `json:"status,omitempty"`
	Provenance      string     `json:"provenance,omitempty"`
	Confidence      string     `json:"confidence,omitempty"`
	FirstObservedAt *time.Time `json:"firstObservedAt,omitempty"`
	LastObservedAt  *time.Time `json:"lastObservedAt,omitempty"`
}

// dependencyView is a direct field-for-field mirror of
// serviceintel.DependencyView. Never omitempty: a dependency present in the
// list is always fully populated by the assembler.
type dependencyView struct {
	Service           string    `json:"service"`
	CallCount         int64     `json:"callCount"`
	ErrorCount        int64     `json:"errorCount"`
	AverageDurationMs int64     `json:"averageDurationMs"`
	FirstObserved     time.Time `json:"firstObserved"`
	LastObserved      time.Time `json:"lastObserved"`
}

// dependenciesView's slices are never nil (see serviceintel.Assembler.Build),
// so neither field carries omitempty -- an empty list must encode as `[]`,
// real information, not be dropped from the response entirely.
type dependenciesView struct {
	Incoming []dependencyView `json:"incoming"`
	Outgoing []dependencyView `json:"outgoing"`
}

// incidentSummaryView mirrors serviceintel.IncidentSummary. Confidence is
// omitempty because it is legitimately empty when RCA has not yet run for
// that incident -- never a fabricated placeholder.
type incidentSummaryView struct {
	IncidentID  string    `json:"incidentId"`
	Status      string    `json:"status"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	StartedAt   time.Time `json:"startedAt"`
	RootService string    `json:"rootService"`
	Confidence  string    `json:"confidence,omitempty"`
}

type serviceIntelligenceResponse struct {
	ServiceName       string                `json:"serviceName"`
	Registry          registryView          `json:"registry"`
	Dependencies      dependenciesView      `json:"dependencies"`
	RelevantIncidents []incidentSummaryView `json:"relevantIncidents"`
	GeneratedAt       time.Time             `json:"generatedAt"`
}

func toServiceIntelligenceResponse(intel serviceintel.ServiceIntelligence) serviceIntelligenceResponse {
	resp := serviceIntelligenceResponse{
		ServiceName:       intel.ServiceName,
		Registry:          registryView{Known: intel.Registry.Known},
		Dependencies:      dependenciesView{Incoming: make([]dependencyView, 0, len(intel.Dependencies.Incoming)), Outgoing: make([]dependencyView, 0, len(intel.Dependencies.Outgoing))},
		RelevantIncidents: make([]incidentSummaryView, 0, len(intel.RelevantIncidents)),
		GeneratedAt:       intel.GeneratedAt,
	}

	if intel.Registry.Known {
		firstObserved := intel.Registry.FirstObservedAt
		lastObserved := intel.Registry.LastObservedAt
		resp.Registry.Status = intel.Registry.Status
		resp.Registry.Provenance = intel.Registry.Provenance
		resp.Registry.Confidence = intel.Registry.Confidence
		resp.Registry.FirstObservedAt = &firstObserved
		resp.Registry.LastObservedAt = &lastObserved
	}

	for _, dep := range intel.Dependencies.Incoming {
		resp.Dependencies.Incoming = append(resp.Dependencies.Incoming, toDependencyViewResponse(dep))
	}
	for _, dep := range intel.Dependencies.Outgoing {
		resp.Dependencies.Outgoing = append(resp.Dependencies.Outgoing, toDependencyViewResponse(dep))
	}

	for _, inc := range intel.RelevantIncidents {
		resp.RelevantIncidents = append(resp.RelevantIncidents, incidentSummaryView{
			IncidentID:  inc.IncidentID,
			Status:      inc.Status,
			Severity:    inc.Severity,
			Title:       inc.Title,
			StartedAt:   inc.StartedAt,
			RootService: inc.RootService,
			Confidence:  inc.Confidence,
		})
	}

	return resp
}

func toDependencyViewResponse(dep serviceintel.DependencyView) dependencyView {
	return dependencyView{
		Service:           dep.Service,
		CallCount:         dep.CallCount,
		ErrorCount:        dep.ErrorCount,
		AverageDurationMs: dep.AverageDurationMs,
		FirstObserved:     dep.FirstObserved,
		LastObserved:      dep.LastObserved,
	}
}

// HandleGetServiceIntelligence handles GET /api/v1/services/{name}/intelligence.
// name is extracted by the caller (see the /api/v1/services/ dispatcher in
// main.go, which mirrors the existing /api/v1/incidents/ dispatcher's
// path-parsing convention). 404 only when name is unknown to the registry,
// the dependency graph, AND incident history all at once -- partial
// evidence from just one or two of those sources is still a real, useful
// 200, never treated as "not found."
func (a *IntelligenceAPI) HandleGetServiceIntelligence(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing service name")
		return
	}

	intel, ok, err := a.assembler.Build(name, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SERVICEINTEL_ERROR", "Failed to build service intelligence")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "No such service is known to the registry, dependency graph, or incident history")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toServiceIntelligenceResponse(intel))
}
