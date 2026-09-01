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
	
	"github.com/atlas/intelligence-engine/internal/aireasoning"
	"github.com/atlas/intelligence-engine/internal/aireasoning/provider"
	"github.com/atlas/intelligence-engine/internal/blast"
	"github.com/atlas/intelligence-engine/internal/evidence"
	"github.com/atlas/intelligence-engine/internal/incidentdetector"
	"github.com/atlas/intelligence-engine/internal/incidentmanager"
	"github.com/atlas/intelligence-engine/internal/incidentsignal"
	"github.com/atlas/intelligence-engine/internal/propagation"
	"github.com/atlas/intelligence-engine/internal/rca"
	"github.com/atlas/intelligence-engine/internal/remediation"
	rmprovider "github.com/atlas/intelligence-engine/internal/remediation/provider"
	"github.com/atlas/intelligence-engine/internal/execution"
	execprovider "github.com/atlas/intelligence-engine/internal/execution/provider"
	"github.com/atlas/intelligence-engine/internal/infrastructure/docker"
	"github.com/atlas/intelligence-engine/internal/registry"
	"github.com/atlas/intelligence-engine/internal/security"
	"github.com/atlas/intelligence-engine/internal/serviceintel"
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

	// Phase 7B: canonical service registry -- a persistent record of "what
	// services are known to this deployment," separate from depGraph's
	// ephemeral, retention-based live topology. See docs/registry.md.
	registryDBPath := os.Getenv("ATLAS_REGISTRY_DB_PATH")
	if registryDBPath == "" {
		registryDBPath = "atlas-registry.db"
	}
	serviceRegistry, err := registry.NewStore(registryDBPath)
	if err != nil {
		slog.Error("Failed to open service registry", slog.String("path", registryDBPath), slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer serviceRegistry.Close()

	registryStaleAfter := 30 * time.Minute
	if s, err := strconv.Atoi(os.Getenv("ATLAS_REGISTRY_STALE_AFTER_SECONDS")); err == nil && s > 0 {
		registryStaleAfter = time.Duration(s) * time.Second
	}
	registryRetireAfter := 24 * time.Hour
	if s, err := strconv.Atoi(os.Getenv("ATLAS_REGISTRY_RETIRE_AFTER_SECONDS")); err == nil && s > 0 {
		registryRetireAfter = time.Duration(s) * time.Second
	}
	if registryRetireAfter <= registryStaleAfter {
		// A registry retire window that isn't strictly longer than the
		// stale window would let a service skip STALE and jump straight to
		// RETIRED, which the documented lifecycle (docs/registry.md)
		// promises never happens on its own -- fail loudly at startup
		// rather than silently accept an inconsistent configuration.
		slog.Error("ATLAS_REGISTRY_RETIRE_AFTER_SECONDS must be greater than ATLAS_REGISTRY_STALE_AFTER_SECONDS",
			slog.Duration("staleAfter", registryStaleAfter), slog.Duration("retireAfter", registryRetireAfter))
		os.Exit(1)
	}

	// M2.4 Initialization
	evStore := evidence.NewStore()
	signalsChan := make(chan incidentsignal.Signal, 10000)
	detectorCfg := incidentdetector.DefaultConfig()
	detector := incidentdetector.NewDetector(detectorCfg, depGraph, signalsChan)
	incManager := incidentmanager.NewManager(incidentmanager.DefaultConfig(), evStore)
	propAnalyzer := propagation.NewAnalyzer(depGraph, corrEngine)
	blastAnalyzer := blast.NewAnalyzer(corrEngine)
	rcaEngine := rca.NewEngine(evStore, propAnalyzer, depGraph)

	// M2.7.1: cross-service incident correlation. Runs before RCA; rca.Engine itself is unmodified.
	correlationWindowStr := os.Getenv("ATLAS_CORRELATION_WINDOW_SECONDS")
	correlationWindow := 20
	if w, err := strconv.Atoi(correlationWindowStr); err == nil && w > 0 {
		correlationWindow = w
	}
	correlator := incidentmanager.NewCorrelator(correlationWindow)

	// M2.7.3: causal evidence attribution. Runs after correlation, before RCA;
	// rca.Engine itself is unmodified. minObservations/DependencyErrorRateThreshold
	// are the SAME values passed to the Detector above (detectorCfg), not a
	// second, independently-declared threshold -- see causal.go's doc comment.
	causalAnalyzer := incidentmanager.NewCausalAnalyzer(detectorCfg.MinObservations, detectorCfg.DependencyErrorRateThreshold)

	// M2.5 Initialization
	aiCfg := aireasoning.Config{
		Enabled:         os.Getenv("ATLAS_AI_ENABLED") != "false",
		Provider:        os.Getenv("ATLAS_AI_PROVIDER"),
		MaxEvents:       200,
		MaxSpans:        200,
		MaxServices:     50,
		MaxAttributes:   50,
		MaxStringLength: 1024,
		TimeoutSeconds:  30,
		RetentionSeconds: 3600,
	}
	var aiProvider aireasoning.ReasoningProvider
	if aiCfg.Provider == "gemini" {
		aiProvider = provider.NewGeminiProvider(os.Getenv("ATLAS_AI_ENDPOINT"), os.Getenv("ATLAS_AI_MODEL"))
	} else {
		// Default to FakeProvider for tests and fallback
		aiProvider = provider.NewFakeProvider()
	}
	aiEngine := aireasoning.NewEngine(aiCfg, aiProvider)

	// M2.6 Initialization
	rmCfg := remediation.Config{
		Enabled:          os.Getenv("ATLAS_AI_ENABLED") != "false",
		Provider:         os.Getenv("ATLAS_AI_PROVIDER"),
		RetentionSeconds: 3600,
	}
	var rmProv remediation.RemediationPlannerProvider
	if rmCfg.Provider == "gemini" {
		rmProv = rmprovider.NewAIPlanner(os.Getenv("ATLAS_AI_ENDPOINT"), os.Getenv("ATLAS_AI_MODEL"))
	} else {
		rmProv = rmprovider.NewFakePlanner()
	}
	rmPlanner := remediation.NewPlanner(rmCfg, rmProv)

	otlpHandler := ingestion.NewOTLPHandler(eventBuffer, corrEngine, detector, serviceRegistry)
	apiHandler := httpapi.NewVerificationAPI(eventBuffer)
	registryAPI := httpapi.NewRegistryAPI(serviceRegistry)

	// Phase 7D: composed, read-only per-service view assembled at request
	// time from the registry, the live dependency graph, and incident
	// history -- see docs/registry.md's "Service Intelligence" section.
	intelligenceAssembler := serviceintel.NewAssembler(serviceRegistry, depGraph, incManager)
	intelligenceAPI := httpapi.NewIntelligenceAPI(intelligenceAssembler)
	corrAPI := httpapi.NewCorrelationAPI(corrEngine)
	graphAPI := httpapi.NewGraphAPI(depGraph)
	incidentAPI := httpapi.NewIncidentAPI(incManager, evStore, rcaEngine, corrEngine, aiEngine, depGraph)
	remediationAPI := httpapi.NewRemediationAPI(incManager, evStore, aiEngine, rmPlanner)

	// M2.7 Initialization
	execEnabled := os.Getenv("ATLAS_EXECUTION_ENABLED") == "true"
	execTimeoutStr := os.Getenv("ATLAS_EXECUTION_TIMEOUT_SECONDS")
	execTimeout := 30
	if t, err := strconv.Atoi(execTimeoutStr); err == nil && t > 0 {
		execTimeout = t
	}

	execGuard := execution.NewGuard(execEnabled)
	execStore := execution.NewStore(retention)
	execVerifier := execution.NewVerifier(incManager, eventBuffer)

	var execProvider execution.ExecutorProvider
	if execEnabled && os.Getenv("ATLAS_EXECUTION_PROVIDER") == "docker" {
		dockerAdapter, err := docker.NewAdapter()
		if err != nil {
			slog.Error("Failed to initialize Docker execution adapter", "error", err)
			os.Exit(1)
		}
		execProvider = dockerAdapter
	} else {
		execProvider = execprovider.NewFakeExecutor()
	}

	execEngine := execution.NewEngine(execGuard, execProvider, execVerifier, execStore, execTimeout)
	executionAPI := httpapi.NewExecutionAPI(execEngine, rmPlanner)

	// M2.9 Initialization: API-key authentication + RBAC. Disabled by
	// default (ATLAS_SECURITY_ENABLED=false), matching this project's
	// existing ATLAS_EXECUTION_ENABLED convention, so pre-M2.9 callers
	// (test-m27-docker.ps1, test-m28-chaos.ps1) are unaffected unless this
	// is explicitly turned on.
	securityEnabled := os.Getenv("ATLAS_SECURITY_ENABLED") == "true"
	apiKeys, err := security.ParseAPIKeys(os.Getenv("ATLAS_API_KEYS"))
	if err != nil {
		slog.Error("Failed to parse ATLAS_API_KEYS", "error", err)
		os.Exit(1)
	}
	authorizer := security.NewAuthorizer(securityEnabled, apiKeys)
	authAPI := httpapi.NewAuthAPI()

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

			// Blast radius first (per-incident, may set AffectedServices from trace data).
			for _, inc := range openIncs {
				blastAnalyzer.Calculate(inc)
			}

			// M2.7.1: correlate cascading incidents across the dependency graph
			// BEFORE RCA runs, so the (unmodified) RCA engine sees the full
			// cascade on the group's primary incident instead of scoring each
			// service in isolation. Metadata-only; never changes Status.
			correlator.Correlate(openIncs, depGraph, now)

			// M2.7.3: re-attribute DEPENDENCY_ERROR evidence from a caller to
			// the callee it actually names, before RCA scores it. rca.Engine
			// itself is unmodified -- see causal.go for the full rationale.
			causalAnalyzer.ApplyCausalAttribution(openIncs, depGraph, evStore)

			for _, inc := range openIncs {
				rcaEngine.Analyze(inc)
				incManager.UpdateIncident(inc)
			}
			
			// Expirations
			corrEngine.CleanupExpired(now)
			depGraph.CleanupExpired(now)
			if err := serviceRegistry.EvaluateLifecycle(now, registryStaleAfter, registryRetireAfter); err != nil {
				slog.Error("Failed to evaluate service registry lifecycle", slog.String("error", err.Error()))
			}
			evStore.CleanupExpired(incidentmanager.DefaultConfig().RetentionSeconds)
			aiEngine.CleanupExpired(now)
			rmPlanner.CleanupExpired(now)
			execEngine.CleanupExpired(now)
		}
	}()

	// Router
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)

	// OTLP Ingestion Endpoints
	mux.HandleFunc("/v1/traces", otlpHandler.HandleTraces)
	mux.HandleFunc("/v1/metrics", otlpHandler.HandleMetrics)

	// Verification APIs (M2.11: read-only telemetry -> PermissionView)
	mux.Handle("/api/v1/events", authorizer.Protect(security.PermissionView, apiHandler.HandleGetEvents))
	mux.Handle("/api/v1/events/metrics", authorizer.Protect(security.PermissionView, apiHandler.HandleGetMetrics))
	mux.Handle("/api/v1/events/trace/", authorizer.Protect(security.PermissionView, apiHandler.HandleGetEventsByTrace))

	// Correlation APIs (M2.11: read-only trace correlation -> PermissionView)
	mux.Handle("/api/v1/correlations/traces/", authorizer.Protect(security.PermissionView, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tree") {
			corrAPI.HandleGetTraceTree(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/timeline") {
			corrAPI.HandleGetTraceTimeline(w, r)
			return
		}
		corrAPI.HandleGetTrace(w, r)
	}))

	// Identity API (Phase 5: read-only "who am I" -> PermissionView, same as
	// every other read endpoint -- every real role holds VIEW).
	mux.Handle("/api/v1/auth/me", authorizer.Protect(security.PermissionView, authAPI.HandleGetMe))

	// Service Registry API (Phase 7B: read-only canonical registry ->
	// PermissionView, same as every other read endpoint). No mutation
	// routes exist -- the registry is only ever written by real observed
	// telemetry (internal/ingestion) or the periodic lifecycle sweep below.
	//
	// Phase 7D: GET /api/v1/services/{name}/intelligence is dispatched
	// inline here, mirroring the /api/v1/incidents/ dispatcher below --
	// everything else falls through unchanged to the existing
	// registryAPI.HandleGetService.
	mux.HandleFunc("/api/v1/services/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		// format: /api/v1/services/{name}/intelligence
		if len(parts) == 6 && parts[5] == "intelligence" {
			name := parts[4]
			authorizer.Protect(security.PermissionView, func(w http.ResponseWriter, r *http.Request) {
				intelligenceAPI.HandleGetServiceIntelligence(w, r, name)
			}).ServeHTTP(w, r)
			return
		}
		authorizer.Protect(security.PermissionView, registryAPI.HandleGetService).ServeHTTP(w, r)
	})
	mux.Handle("/api/v1/services", authorizer.Protect(security.PermissionView, registryAPI.HandleListServices))

	// Graph APIs (M2.11: read-only dependency graph -> PermissionView)
	mux.Handle("/api/v1/graph/services/", authorizer.Protect(security.PermissionView, graphAPI.HandleGetServiceDependencies))
	mux.Handle("/api/v1/graph/edges", authorizer.Protect(security.PermissionView, graphAPI.HandleGetEdges))
	mux.Handle("/api/v1/graph", authorizer.Protect(security.PermissionView, graphAPI.HandleGetGraph))

	// Incident APIs (M2.11: reads -> PermissionView; execution/audit history -> PermissionReadAudit)
	mux.HandleFunc("/api/v1/incidents/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if r.URL.Path == "/api/v1/incidents/open" {
			authorizer.Protect(security.PermissionView, incidentAPI.HandleGetOpenIncidents).ServeHTTP(w, r)
			return
		}
		// Route remediation paths natively
		// format: /api/v1/incidents/{incidentId}/remediation...
		if len(parts) >= 6 && parts[5] == "remediation" {
			id := parts[4]
			if len(parts) == 7 && parts[6] == "plan" {
				if r.Method == http.MethodPost {
					authorizer.Protect(security.PermissionCreatePlan, func(w http.ResponseWriter, r *http.Request) {
						remediationAPI.HandlePostPlan(w, r, id)
					}).ServeHTTP(w, r)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
				return
			}
			if r.Method == http.MethodGet {
				authorizer.Protect(security.PermissionView, func(w http.ResponseWriter, r *http.Request) {
					remediationAPI.HandleGetPlanByIncident(w, r, id)
				}).ServeHTTP(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if len(parts) >= 6 && parts[5] == "executions" {
			if r.Method == http.MethodGet {
				authorizer.Protect(security.PermissionReadAudit, executionAPI.HandleGetExecutionsByIncident).ServeHTTP(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		// Module 4: POST /{incidentId}/analyze triggers AI reasoning.
		// Previously misrouted into HandleGetIncident (a GET-only handler --
		// see incident.go's own top-of-function method guard), which meant
		// this call always returned 405 and was also left unauthenticated
		// even when ATLAS_SECURITY_ENABLED=true (an M2.11-era gap explicitly
		// deferred, not a permanent decision -- see docs/ai-reasoning.md).
		// Routed directly to the real, already-complete HandlePostAnalyze
		// handler, gated by the same PermissionView its sibling
		// GET .../analysis already requires -- this is advisory read/insight
		// output, not a mutation of Incident/RemediationPlan/execution state.
		if len(parts) >= 6 && parts[5] == "analyze" && r.Method == http.MethodPost {
			id := parts[4]
			authorizer.Protect(security.PermissionView, func(w http.ResponseWriter, r *http.Request) {
				incidentAPI.HandlePostAnalyze(w, r, id)
			}).ServeHTTP(w, r)
			return
		}
		authorizer.Protect(security.PermissionView, incidentAPI.HandleGetIncident).ServeHTTP(w, r)
	})
	mux.Handle("/api/v1/incidents", authorizer.Protect(security.PermissionView, incidentAPI.HandleGetIncidents))

	// Remediation API (/api/v1/remediation/...)
	mux.HandleFunc("/api/v1/remediation/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 6 {
			if parts[5] == "approve" && r.Method == http.MethodPost {
				authorizer.Protect(security.PermissionApprovePlan, remediationAPI.HandleApprove).ServeHTTP(w, r)
				return
			}
			if parts[5] == "reject" && r.Method == http.MethodPost {
				authorizer.Protect(security.PermissionApprovePlan, remediationAPI.HandleReject).ServeHTTP(w, r)
				return
			}
			if parts[5] == "execute" && r.Method == http.MethodPost {
				authorizer.Protect(security.PermissionExecute, executionAPI.HandleExecute).ServeHTTP(w, r)
				return
			}
		}
		if r.Method == http.MethodGet {
			authorizer.Protect(security.PermissionView, remediationAPI.HandleGetPlan).ServeHTTP(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	// Execution API (/api/v1/executions/...) -- execution/audit history -> PermissionReadAudit
	mux.HandleFunc("/api/v1/executions/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			authorizer.Protect(security.PermissionReadAudit, executionAPI.HandleGetExecution).ServeHTTP(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	})

	mux.Handle("/api/v1/events/", authorizer.Protect(security.PermissionView, func(w http.ResponseWriter, r *http.Request) {
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
	}))

	// M2.15 dashboard: minimal CORS so the separate-origin frontend (Vite
	// dev server by default) can call this API from a browser. Defaults to
	// Vite's default local dev port; empty ATLAS_CORS_ORIGIN disables CORS
	// headers entirely (see httpapi.WithCORS).
	corsOrigin := os.Getenv("ATLAS_CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:5173"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: httpapi.WithCORS(mux, corsOrigin),
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
