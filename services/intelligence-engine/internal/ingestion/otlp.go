package ingestion

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/incidentdetector"
	"github.com/atlas/intelligence-engine/internal/normalization"
	"github.com/atlas/intelligence-engine/internal/registry"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

type OTLPHandler struct {
	buffer            *buffer.EventBuffer
	correlationEngine *correlation.Engine
	detector          *incidentdetector.Detector
	// registry is the Phase 7B canonical service registry. Observing here
	// (rather than inside normalization or correlation) keeps the registry
	// entirely additive: it reads the same already-normalized ServiceName
	// every event carries, and its own failure/absence never affects the
	// existing buffer/correlation/detection path below.
	registry *registry.Store
}

func NewOTLPHandler(b *buffer.EventBuffer, c *correlation.Engine, d *incidentdetector.Detector, reg *registry.Store) *OTLPHandler {
	return &OTLPHandler{buffer: b, correlationEngine: c, detector: d, registry: reg}
}

// observeServices upserts every distinct real service.name seen in one
// ingestion batch into the canonical registry, deduplicated so a
// multi-span trace payload does not issue one registry write per span.
// Never invents a name: each entry is exactly one event's already
// OTel-resource-derived ServiceName (see normalization.getServiceName).
func (h *OTLPHandler) observeServices(serviceNames []string) {
	if h.registry == nil {
		return
	}
	seen := make(map[string]bool, len(serviceNames))
	now := time.Now()
	for _, name := range serviceNames {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if err := h.registry.Observe(name, now); err != nil {
			slog.Error("Failed to update service registry", slog.String("service", name), slog.String("error", err.Error()))
		}
	}
}

// HandleTraces processes POST /v1/traces
func (h *OTLPHandler) HandleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reader io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			slog.Error("Failed to create gzip reader", slog.String("error", err.Error()))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		defer gz.Close()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		slog.Error("Failed to read OTLP trace body", slog.String("error", err.Error()))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req tracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		slog.Error("Failed to unmarshal OTLP trace", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	events := normalization.NormalizeTraces(req.GetResourceSpans())
	slog.Info("Processed trace payload", "events_count", len(events))
	serviceNames := make([]string, 0, len(events))
	for _, e := range events {
		h.buffer.Add(e)
		h.correlationEngine.ProcessEvent(e)
		h.detector.ProcessEvent(e)
		serviceNames = append(serviceNames, e.ServiceName)
	}
	h.observeServices(serviceNames)

	w.WriteHeader(http.StatusOK)
}

// HandleMetrics processes POST /v1/metrics
func (h *OTLPHandler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reader io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			slog.Error("Failed to create gzip reader", slog.String("error", err.Error()))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		defer gz.Close()
		reader = gz
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		slog.Error("Failed to read OTLP metric body", slog.String("error", err.Error()))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req metricpb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &req); err != nil {
		slog.Error("Failed to unmarshal OTLP metric", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	events := normalization.NormalizeMetrics(req.GetResourceMetrics())
	serviceNames := make([]string, 0, len(events))
	for _, e := range events {
		h.buffer.Add(e)
		serviceNames = append(serviceNames, e.ServiceName)
	}
	h.observeServices(serviceNames)

	w.WriteHeader(http.StatusOK)
}
