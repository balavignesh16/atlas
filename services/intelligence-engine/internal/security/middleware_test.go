package security

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestAuthorizer(t *testing.T, enabled bool) *Authorizer {
	t.Helper()
	ks, err := ParseAPIKeys("alice:alice-secret-key:OPERATOR,bob:bob-secret-key:APPROVER,carol:carol-secret-key:EXECUTOR,view1:view1-secret-key:VIEWER,root:root-secret-key:ADMIN")
	if err != nil {
		t.Fatalf("unexpected keystore parse error: %v", err)
	}
	return NewAuthorizer(enabled, ks)
}

func newEchoHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if p, ok := FromContext(r.Context()); ok {
			w.Write([]byte("principal:" + p.Name))
			return
		}
		w.Write([]byte("no-principal"))
	}
}

func TestAuthenticate_MissingKey_Returns401(t *testing.T) {
	az := newTestAuthorizer(t, true)
	handler := az.Authenticate(newEchoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthenticate_InvalidKey_Returns401(t *testing.T) {
	az := newTestAuthorizer(t, true)
	handler := az.Authenticate(newEchoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(APIKeyHeader, "not-a-configured-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthenticate_ValidKey_AttachesPrincipalAndCalls200(t *testing.T) {
	az := newTestAuthorizer(t, true)
	handler := az.Authenticate(newEchoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(APIKeyHeader, "alice-secret-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Result().Body)
	if string(body) != "principal:alice" {
		t.Fatalf("expected the handler to see the resolved principal, got %q", string(body))
	}
}

func TestAuthenticate_Disabled_PassesThroughWithoutPrincipal(t *testing.T) {
	az := newTestAuthorizer(t, false)
	handler := az.Authenticate(newEchoHandler(t))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Deliberately no API key header at all -- disabled means this must
	// still succeed, matching pre-M2.9 behavior exactly.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when security is disabled, got %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Result().Body)
	if string(body) != "no-principal" {
		t.Fatalf("expected no principal attached when disabled, got %q", string(body))
	}
}

func TestAuthenticate_ErrorResponse_NeverContainsThePresentedKey(t *testing.T) {
	az := newTestAuthorizer(t, true)
	handler := az.Authenticate(newEchoHandler(t))

	secretProbe := "definitely-not-a-real-key-xyz123"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(APIKeyHeader, secretProbe)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Result().Body)
	if strings.Contains(string(body), secretProbe) {
		t.Fatalf("the presented API key must never be echoed back in the error response, got body: %s", string(body))
	}
}

func TestRequirePermission_UnauthorizedRole_Returns403(t *testing.T) {
	az := newTestAuthorizer(t, true)
	// alice is OPERATOR: has CREATE_PLAN, not EXECUTE.
	handler := az.Protect(PermissionExecute, newEchoHandler(t))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(APIKeyHeader, "alice-secret-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for an OPERATOR calling an EXECUTE-gated endpoint, got %d", rec.Code)
	}
}

func TestRequirePermission_AuthorizedRole_Returns200(t *testing.T) {
	az := newTestAuthorizer(t, true)
	// carol is EXECUTOR: has EXECUTE.
	handler := az.Protect(PermissionExecute, newEchoHandler(t))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(APIKeyHeader, "carol-secret-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an EXECUTOR calling an EXECUTE-gated endpoint, got %d", rec.Code)
	}
}

func TestRequirePermission_Disabled_PassesThroughRegardlessOfPermission(t *testing.T) {
	az := newTestAuthorizer(t, false)
	handler := az.Protect(PermissionExecute, newEchoHandler(t))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	// No key at all -- disabled must still allow this through.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when security is disabled regardless of permission, got %d", rec.Code)
	}
}

func TestRequirePermission_ViewerCannotCreateApproveExecute(t *testing.T) {
	az := newTestAuthorizer(t, true)

	for _, perm := range []Permission{PermissionCreatePlan, PermissionApprovePlan, PermissionExecute} {
		handler := az.Protect(perm, newEchoHandler(t))
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(APIKeyHeader, "view1-secret-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected VIEWER to be forbidden from %s, got status %d", perm, rec.Code)
		}
	}
}

func TestRequirePermission_AdminCanPerformEveryPermission(t *testing.T) {
	az := newTestAuthorizer(t, true)

	for _, perm := range []Permission{PermissionView, PermissionCreatePlan, PermissionApprovePlan, PermissionExecute, PermissionReadAudit} {
		handler := az.Protect(perm, newEchoHandler(t))
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set(APIKeyHeader, "root-secret-key")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected ADMIN to be allowed %s, got status %d", perm, rec.Code)
		}
	}
}

func TestFromContext_NoPrincipalAttached(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := FromContext(req.Context()); ok {
		t.Fatal("expected a bare request context to carry no principal")
	}
}

func TestWithPrincipal_RoundTrips(t *testing.T) {
	ctx := WithPrincipal(httptest.NewRequest(http.MethodGet, "/", nil).Context(), Principal{Name: "dave", Role: RoleAdmin})
	p, ok := FromContext(ctx)
	if !ok || p.Name != "dave" || p.Role != RoleAdmin {
		t.Fatalf("expected round-tripped principal dave/ADMIN, got %+v ok=%v", p, ok)
	}
}
