package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/atlas/intelligence-engine/internal/registry"
)

// RegistryAPI exposes the canonical service registry read-only. There are
// deliberately no mutation endpoints: the only way a service enters or
// changes state in the registry is real evidence (registry.Store.Record,
// called from internal/ingestion) or the periodic lifecycle sweep -- never
// a direct API write, which would let a caller fabricate a service Atlas
// never actually saw evidence for.
type RegistryAPI struct {
	store *registry.Store
}

func NewRegistryAPI(store *registry.Store) *RegistryAPI {
	return &RegistryAPI{store: store}
}

// Mirrors registry.Service field-for-field with JSON tags, plus Confidence
// (Phase 7C), which is derived from Provenance rather than stored --
// registry.ConfidenceFor is a pure function of the enum, not a fabricated
// per-record score.
type serviceResponse struct {
	Name            string     `json:"name"`
	DisplayName     string     `json:"displayName"`
	Provenance      string     `json:"provenance"`
	Confidence      string     `json:"confidence"`
	Status          string     `json:"status"`
	FirstObservedAt time.Time  `json:"firstObservedAt"`
	LastObservedAt  time.Time  `json:"lastObservedAt"`
	LastTelemetryAt *time.Time `json:"lastTelemetryAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func toServiceResponse(svc registry.Service) serviceResponse {
	return serviceResponse{
		Name:            svc.Name,
		DisplayName:     svc.DisplayName,
		Provenance:      string(svc.Provenance),
		Confidence:      string(registry.ConfidenceFor(svc.Provenance)),
		Status:          string(svc.Status),
		FirstObservedAt: svc.FirstObservedAt,
		LastObservedAt:  svc.LastObservedAt,
		LastTelemetryAt: svc.LastTelemetryAt,
		CreatedAt:       svc.CreatedAt,
		UpdatedAt:       svc.UpdatedAt,
	}
}

var validStatuses = map[string]registry.Status{
	string(registry.StatusActive):  registry.StatusActive,
	string(registry.StatusStale):   registry.StatusStale,
	string(registry.StatusRetired): registry.StatusRetired,
}

var validSources = map[string]registry.Provenance{
	string(registry.ProvenanceObservedTelemetry): registry.ProvenanceObservedTelemetry,
	string(registry.ProvenanceDeclared):          registry.ProvenanceDeclared,
	string(registry.ProvenanceDocker):            registry.ProvenanceDocker,
	string(registry.ProvenanceKubernetes):        registry.ProvenanceKubernetes,
	string(registry.ProvenanceConfig):            registry.ProvenanceConfig,
	string(registry.ProvenanceInferred):          registry.ProvenanceInferred,
}

// HandleListServices handles GET /api/v1/services?status=&source=&q= --
// every known service, ACTIVE/STALE/RETIRED alike, optionally narrowed.
// This is the canonical registry, not the live telemetry graph: a service
// can appear here long after its graph node has expired under
// DependencyGraph's own, unrelated retention.
func (a *RegistryAPI) HandleListServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := registry.ListFilter{Query: strings.TrimSpace(r.URL.Query().Get("q"))}

	if raw := r.URL.Query().Get("status"); raw != "" {
		status, ok := validStatuses[strings.ToUpper(raw)]
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_STATUS", "status must be one of ACTIVE, STALE, RETIRED")
			return
		}
		filter.Status = &status
	}

	if raw := r.URL.Query().Get("source"); raw != "" {
		source, ok := validSources[strings.ToUpper(raw)]
		if !ok {
			writeError(w, http.StatusBadRequest, "INVALID_SOURCE", "source must be a known provenance value")
			return
		}
		filter.Source = &source
	}

	services, err := a.store.List(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTRY_ERROR", "Failed to list services")
		return
	}

	responses := make([]serviceResponse, 0, len(services))
	for _, svc := range services {
		responses = append(responses, toServiceResponse(svc))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responses)
}

// HandleGetService handles GET /api/v1/services/{name}.
func (a *RegistryAPI) HandleGetService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/v1/services/")
	if name == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "Missing service name")
		return
	}

	svc, ok, err := a.store.Get(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTRY_ERROR", "Failed to load service")
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "SERVICE_NOT_FOUND", "No such service is known to the registry")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toServiceResponse(svc))
}
