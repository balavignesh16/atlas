package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/event"
)

type VerificationAPI struct {
	buffer *buffer.EventBuffer
}

func NewVerificationAPI(b *buffer.EventBuffer) *VerificationAPI {
	return &VerificationAPI{buffer: b}
}

// HandleGetEvents GET /api/v1/events
func (api *VerificationAPI) HandleGetEvents(w http.ResponseWriter, r *http.Request) {
	events := api.buffer.GetAll()
	writeJSON(w, events)
}

// HandleGetEventByID GET /api/v1/events/{eventId}
func (api *VerificationAPI) HandleGetEventByID(w http.ResponseWriter, r *http.Request) {
	// Simple path parsing since we are not using a heavy router in M2.2
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/events/")
	if path == "" || strings.Contains(path, "/") {
		http.NotFound(w, r)
		return
	}

	events := api.buffer.GetAll()
	for _, e := range events {
		if e.EventID == path {
			writeJSON(w, e)
			return
		}
	}
	http.NotFound(w, r)
}

// HandleGetEventsByTrace GET /api/v1/events/trace/{traceId}
func (api *VerificationAPI) HandleGetEventsByTrace(w http.ResponseWriter, r *http.Request) {
	traceID := strings.TrimPrefix(r.URL.Path, "/api/v1/events/trace/")
	if traceID == "" || strings.Contains(traceID, "/") {
		http.NotFound(w, r)
		return
	}

	all := api.buffer.GetAll()
	var matched []event.ATLASEvent
	for _, e := range all {
		if e.EventType == event.EventTypeTraceSpan && e.TraceID == traceID {
			matched = append(matched, e)
		}
	}

	// Sort by start time (Timestamp)
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Timestamp.Before(matched[j].Timestamp)
	})

	writeJSON(w, matched)
}

// HandleGetMetrics GET /api/v1/events/metrics
func (api *VerificationAPI) HandleGetMetrics(w http.ResponseWriter, r *http.Request) {
	all := api.buffer.GetAll()
	var matched []event.ATLASEvent
	for _, e := range all {
		if e.EventType == event.EventTypeMetric {
			matched = append(matched, e)
		}
	}

	writeJSON(w, matched)
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode json", http.StatusInternalServerError)
	}
}
