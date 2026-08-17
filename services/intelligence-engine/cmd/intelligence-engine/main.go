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
	"github.com/atlas/intelligence-engine/internal/httpapi"
	"github.com/atlas/intelligence-engine/internal/ingestion"
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
	otlpHandler := ingestion.NewOTLPHandler(eventBuffer)
	apiHandler := httpapi.NewVerificationAPI(eventBuffer)

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
