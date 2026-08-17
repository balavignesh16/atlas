package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/atlas/intelligence-engine/internal/buffer"
	"github.com/atlas/intelligence-engine/internal/correlation"
	"github.com/atlas/intelligence-engine/internal/graph"
	"github.com/atlas/intelligence-engine/internal/httpapi"
	"github.com/atlas/intelligence-engine/internal/ingestion"
	
	"github.com/atlas/intelligence-engine/internal/blast"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentdetector"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/propagation"
	"github.com/atlas/intelligence-engine/internal/rca"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "UP"})
}

func main() {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		slog.String("service", "atlas-intelligence-engine"),
		slog.String("environment", env),
	)
	slog.SetDefault(logger)

	port := os.Getenv("ATLAS_ENGINE_PORT")
	if port == "" {
		port = "8081"
	}

	capacityStr := os.Getenv("ATLAS_EVENT_BUFFER_CAPACITY")
	capacity := 10000
	if c, err := strconv.Atoi(capacityStr); err == nil && c > 0 {
		capacity = c
	}

	slog.Info("Initializing Intelligence Engine", "capacity", capacity)

	// Components
	eventBuffer := buffer.NewEventBuffer(capacity)
	
	retentionStr := os.Getenv("ATLAS_CORRELATION_RETENTION_SECONDS")
	retention := 300
	if r, err := strconv.Atoi(retentionStr); err == nil && r > 0 {
		retention = r
	}

	depGraph := graph.NewDependencyGraph(retention)
	corrEngine := correlation.NewEngine(depGraph, retention)

	// M2.4 Initialization
	evStore := evidence.NewStore()
	signalsChan := make(chan incidentsignal.Signal, 10000)
	detector := incidentdetector.NewDetector(incidentdetector.DefaultConfig(), depGraph, signalsChan)
	incManager := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evStore)
	propAnalyzer := propagation.NewAnalyzer(depGraph, corrEngine)
	blastAnalyzer := blast.NewAnalyzer(corrEngine)
	rcaEngine := rca.NewEngine(evStore, propAnalyzer, depGraph)

	otlpHandler := ingestion.NewOTLPHandler(eventBuffer, corrEngine, detector)
	apiHandler := httpapi.NewVerificationAPI(eventBuffer)
	corrAPI := httpapi.NewCorrelationAPI(corrEngine)
	graphAPI := httpapi.NewGraphAPI(depGraph)
	incidentAPI := httpapi.NewIncidentAPI(incManager, evStore, rcaEngine, corrEngine)

	go func() {
		for sig := range signalsChan {
			incManager.ProcessSignal(sig)
		}
	}()

	// Background cleanup
	go func() {
		ticker := time.NewTicker(5 * time.Second) // ATLAS_INCIDENT_EVALUATION_INTERVAL_SECONDS
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			
			// Detectors and Managers
			detector.EvaluateAll()
			incManager.CleanupAndResolve()
			
			// Re-evaluate incidents
			openIncs := incManager.GetOpenIncidents()
			for _, inc := range openIncs {
				blastAnalyzer.Calculate(inc)
				rcaEngine.Analyze(inc)
				incManager.UpdateIncident(inc)
			}
			
			// Expirations
			corrEngine.CleanupExpired(now)
			depGraph.CleanupExpired(now)
			evStore.CleanupExpired(incidentmanager.DefaultConfig().RetentionSeconds)
		}
	}()

	// Router
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	// OTLP Ingestion Endpoints
	mux.HandleFunc("/v1/traces", otlpHandler.HandleTraces)
	mux.HandleFunc("/v1/metrics", otlpHandler.HandleMetrics)

	// Verification APIs
	mux.HandleFunc("/api/v1/events", apiHandler.HandleGetEvents)
	mux.HandleFunc("/api/v1/events/metrics", apiHandler.HandleGetMetrics)
	mux.HandleFunc("/api/v1/events/trace/", apiHandler.HandleGetEventsByTrace)

	// Correlation APIs
	mux.HandleFunc("/api/v1/correlations/traces/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tree") {
			corrAPI.HandleGetTraceTree(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/timeline") {
			corrAPI.HandleGetTraceTimeline(w, r)
			return
		}
		corrAPI.HandleGetTrace(w, r)
	})

	// Graph APIs
	mux.HandleFunc("/api/v1/graph/services/", graphAPI.HandleGetServiceDependencies)
	mux.HandleFunc("/api/v1/graph/edges", graphAPI.HandleGetEdges)
	mux.HandleFunc("/api/v1/graph", graphAPI.HandleGetGraph)

	// Incident APIs
	mux.HandleFunc("/api/v1/incidents/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/incidents/open" {
			incidentAPI.HandleGetOpenIncidents(w, r)
			return
		}
		incidentAPI.HandleGetIncident(w, r)
	})
	mux.HandleFunc("/api/v1/incidents", incidentAPI.HandleGetIncidents)

	mux.HandleFunc("/api/v1/events/", func(w http.ResponseWriter, r *http.Request) {
		// Route disambiguation since /api/v1/events/ prefix matches both ID and Trace
		if strings.HasPrefix(r.URL.Path, "/api/v1/events/trace/") {
			apiHandler.HandleGetEventsByTrace(w, r)
			return
		}
		if r.URL.Path == "/api/v1/events/metrics" {
			apiHandler.HandleGetMetrics(w, r)
			return
		}
		if r.URL.Path == "/api/v1/events" || r.URL.Path == "/api/v1/events/" {
			apiHandler.HandleGetEvents(w, r)
			return
		}
		// Fallback to GetEventByID
		apiHandler.HandleGetEventByID(w, r)
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		slog.Info("Starting server", slog.String("port", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", slog.String("error", err.Error()))
	}

	slog.Info("Server exiting")
}
