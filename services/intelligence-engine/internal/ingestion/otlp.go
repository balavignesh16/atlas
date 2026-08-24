package ingestion

import (
	"compress/gzip"
	"io"
	"log/slog"
	"net/http"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/incidentdetector"
	"github.com/atlas/intelligence-engine/internal/normalization"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

type OTLPHandler struct {
	buffer            *buffer.EventBuffer
	correlationEngine *correlation.Engine
	detector          *incidentdetector.Detector
}

func NewOTLPHandler(b *buffer.EventBuffer, c *correlation.Engine, d *incidentdetector.Detector) *OTLPHandler {
	return &OTLPHandler{buffer: b, correlationEngine: c, detector: d}
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
	for _, e := range events {
		h.buffer.Add(e)
		h.correlationEngine.ProcessEvent(e)
		h.detector.ProcessEvent(e)
	}

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
	for _, e := range events {
		h.buffer.Add(e)
	}

	w.WriteHeader(http.StatusOK)
}
