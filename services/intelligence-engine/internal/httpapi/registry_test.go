package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atlas/intelligence-engine/internal/registry"
)

func newTestRegistryAPI(t *testing.T) (*RegistryAPI, *registry.Store) {
	t.Helper()
	store, err := registry.NewStore(":memory:")
	if err != nil {
		t.Fatalf("registry.NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return NewRegistryAPI(store), store
}

func TestHandleListServices_WrongMethod_Returns405(t *testing.T) {
	api, _ := newTestRegistryAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleListServices_Empty_ReturnsEmptyArrayNotNull(t *testing.T) {
	api, _ := newTestRegistryAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body != "[]\n" {
		t.Fatalf("expected an empty JSON array (not null) when no services are registered, got %q", body)
	}
}

func TestHandleListServices_ReturnsRegisteredServices(t *testing.T) {
	api, store := newTestRegistryAPI(t)
	if err := store.Observe("checkout-service", time.Now()); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 service, got %d", len(got))
	}
	if got[0]["name"] != "checkout-service" {
		t.Errorf("name = %v, want checkout-service", got[0]["name"])
	}
	if got[0]["provenance"] != "OBSERVED_TELEMETRY" {
		t.Errorf("provenance = %v, want OBSERVED_TELEMETRY", got[0]["provenance"])
	}
	if got[0]["confidence"] != "OBSERVED" {
		t.Errorf("confidence = %v, want OBSERVED (OBSERVED_TELEMETRY's fixed confidence class)", got[0]["confidence"])
	}
	if got[0]["status"] != "ACTIVE" {
		t.Errorf("status = %v, want ACTIVE", got[0]["status"])
	}
	// No fabricated metadata: only the documented fields should be present.
	allowed := map[string]bool{
		"name": true, "displayName": true, "provenance": true, "confidence": true, "status": true,
		"firstObservedAt": true, "lastObservedAt": true, "lastTelemetryAt": true,
		"createdAt": true, "updatedAt": true,
	}
	for key := range got[0] {
		if !allowed[key] {
			t.Errorf("unexpected field %q in service response -- no field beyond the documented registry model should be exposed", key)
		}
	}
}

func TestHandleGetService_Found(t *testing.T) {
	api, store := newTestRegistryAPI(t)
	if err := store.Observe("checkout-service", time.Now()); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/checkout-service", nil)
	rec := httptest.NewRecorder()
	api.HandleGetService(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got["name"] != "checkout-service" {
		t.Errorf("name = %v, want checkout-service", got["name"])
	}
}

func TestHandleGetService_UnknownService_Returns404(t *testing.T) {
	api, _ := newTestRegistryAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services/never-seen", nil)
	rec := httptest.NewRecorder()
	api.HandleGetService(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown service, got %d", rec.Code)
	}
}

func TestHandleGetService_WrongMethod_Returns405(t *testing.T) {
	api, _ := newTestRegistryAPI(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/checkout-service", nil)
	rec := httptest.NewRecorder()
	api.HandleGetService(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func seedTwoServices(t *testing.T, store *registry.Store) {
	t.Helper()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.Record(registry.Evidence{ServiceName: "checkout-service", Source: registry.ProvenanceObservedTelemetry, ObservedAt: at}); err != nil {
		t.Fatalf("seed checkout-service: %v", err)
	}
	if err := store.Record(registry.Evidence{ServiceName: "legacy-worker", Source: registry.ProvenanceDeclared, ObservedAt: at}); err != nil {
		t.Fatalf("seed legacy-worker: %v", err)
	}
}

func TestHandleListServices_FilterByStatus(t *testing.T) {
	api, store := newTestRegistryAPI(t)
	seedTwoServices(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?status=ACTIVE", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected both services to be ACTIVE, got %d: %+v", len(got), got)
	}
}

func TestHandleListServices_FilterBySource(t *testing.T) {
	api, store := newTestRegistryAPI(t)
	seedTwoServices(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?source=DECLARED", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0]["name"] != "legacy-worker" {
		t.Fatalf("expected only legacy-worker for source=DECLARED, got %+v", got)
	}
}

func TestHandleListServices_FilterByQuery(t *testing.T) {
	api, store := newTestRegistryAPI(t)
	seedTwoServices(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?q=checkout", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	var got []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0]["name"] != "checkout-service" {
		t.Fatalf("expected only checkout-service for q=checkout, got %+v", got)
	}
}

func TestHandleListServices_FilterByQuery_NoMatch_ReturnsEmptyArray(t *testing.T) {
	api, store := newTestRegistryAPI(t)
	seedTwoServices(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?q=zzz-nonexistent", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a query with no matches, got %d", rec.Code)
	}
	if rec.Body.String() != "[]\n" {
		t.Fatalf("expected an empty array for a non-matching query, got %q", rec.Body.String())
	}
}

func TestHandleListServices_InvalidStatus_Returns400(t *testing.T) {
	api, _ := newTestRegistryAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?status=NOT_A_REAL_STATUS", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid status filter, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListServices_InvalidSource_Returns400(t *testing.T) {
	api, _ := newTestRegistryAPI(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?source=NOT_A_REAL_SOURCE", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid source filter, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleListServices_StatusFilterIsCaseInsensitive(t *testing.T) {
	api, store := newTestRegistryAPI(t)
	seedTwoServices(t, store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/services?status=active", nil)
	rec := httptest.NewRecorder()
	api.HandleListServices(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected lowercase status value to be accepted, got %d", rec.Code)
	}
}
