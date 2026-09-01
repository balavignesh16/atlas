package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atlas/intelligence-engine/internal/security"
)

func TestHandleGetMe_WrongMethod_Returns405(t *testing.T) {
	api := NewAuthAPI()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	api.HandleGetMe(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleGetMe_NoPrincipalInContext_ReportsSecurityDisabled(t *testing.T) {
	// The only way this handler runs without a Principal in context is when
	// ATLAS_SECURITY_ENABLED=false (Authorizer.Protect is then a pure
	// pass-through) -- a missing/invalid key with security ON is rejected
	// with 401 by the middleware before this handler ever runs.
	api := NewAuthAPI()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	api.HandleGetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got identityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if got.SecurityEnabled {
		t.Fatalf("expected securityEnabled=false with no principal in context, got true")
	}
	if got.Name != "" || got.Role != "" || len(got.Permissions) != 0 {
		t.Fatalf("expected no identity fields when security is disabled, got %+v", got)
	}
}

func TestHandleGetMe_WithPrincipal_ReturnsRealNameRoleAndPermissions(t *testing.T) {
	api := NewAuthAPI()
	principal := security.Principal{Name: "alice", Role: security.RoleOperator}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(security.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()
	api.HandleGetMe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var got identityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if !got.SecurityEnabled {
		t.Fatalf("expected securityEnabled=true with a principal in context")
	}
	if got.Name != "alice" {
		t.Fatalf("expected name %q, got %q", "alice", got.Name)
	}
	if got.Role != "OPERATOR" {
		t.Fatalf("expected role %q, got %q", "OPERATOR", got.Role)
	}
	// OPERATOR holds exactly VIEW and CREATE_PLAN (internal/security/model.go).
	want := map[string]bool{"VIEW": true, "CREATE_PLAN": true}
	if len(got.Permissions) != len(want) {
		t.Fatalf("expected %d permissions, got %v", len(want), got.Permissions)
	}
	for _, p := range got.Permissions {
		if !want[p] {
			t.Fatalf("unexpected permission %q for OPERATOR: %v", p, got.Permissions)
		}
	}
}

func TestHandleGetMe_AdminHasAllFivePermissions(t *testing.T) {
	api := NewAuthAPI()
	principal := security.Principal{Name: "root", Role: security.RoleAdmin}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req = req.WithContext(security.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()
	api.HandleGetMe(rec, req)

	var got identityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if len(got.Permissions) != 5 {
		t.Fatalf("expected ADMIN to hold all 5 permissions, got %v", got.Permissions)
	}
}

func TestHandleGetMe_NeverExposesTheAPIKey(t *testing.T) {
	api := NewAuthAPI()
	principal := security.Principal{Name: "alice", Role: security.RoleOperator}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set(security.APIKeyHeader, "super-secret-key-should-never-appear")
	req = req.WithContext(security.WithPrincipal(req.Context(), principal))
	rec := httptest.NewRecorder()
	api.HandleGetMe(rec, req)

	body := rec.Body.String()
	if want := "super-secret-key-should-never-appear"; strings.Contains(body, want) {
		t.Fatalf("response body must never contain the API key, got body: %s", body)
	}
}
