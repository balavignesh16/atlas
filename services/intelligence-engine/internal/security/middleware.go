package security

import (
	"encoding/json"
	"net/http"
)

// APIKeyHeader is the HTTP header protected endpoints expect the caller's
// API key in. Documented here as the single source of truth for the header
// name, rather than repeating the literal string at each call site.
const APIKeyHeader = "X-Atlas-Api-Key"

// Authorizer owns both authentication (resolving an API key to a Principal)
// and authorization (checking a Principal's permission), so the two stages
// share one enabled/disabled state and can never drift out of sync with
// each other. Handlers only ever ask for RequirePermission(x); they never
// see KeyStore or Role directly.
type Authorizer struct {
	enabled bool
	keys    *KeyStore
}

// NewAuthorizer constructs an Authorizer. enabled corresponds to
// ATLAS_SECURITY_ENABLED (default false, matching this project's existing
// ATLAS_EXECUTION_ENABLED convention): when false, every method below is a
// pass-through no-op, so pre-M2.9 callers (test-m27-docker.ps1,
// test-m28-chaos.ps1) are byte-for-byte unaffected.
func NewAuthorizer(enabled bool, keys *KeyStore) *Authorizer {
	return &Authorizer{enabled: enabled, keys: keys}
}

// Authenticate resolves the request's API key to a Principal and attaches
// it to the request context. When disabled, it is a pure pass-through and
// attaches no principal at all -- downstream handlers must treat "no
// principal in context" as "fall back to pre-M2.9 behavior," never as "an
// anonymous principal."
func (a *Authorizer) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.enabled {
			next.ServeHTTP(w, r)
			return
		}

		key := r.Header.Get(APIKeyHeader)
		if key == "" {
			writeSecurityError(w, http.StatusUnauthorized, "missing API key")
			return
		}

		principal, ok := a.keys.Lookup(key)
		if !ok {
			writeSecurityError(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), principal)))
	})
}

// RequirePermission returns middleware that rejects the request with 403
// unless the authenticated Principal holds permission. When the Authorizer
// is disabled, it is a pure pass-through, matching Authenticate's
// no-op behavior in the same state. Must be composed after Authenticate
// (Authenticate attaches the Principal that RequirePermission reads); if a
// principal is somehow missing while enabled, this fails closed with 401
// rather than assuming Authenticate already ran.
func (a *Authorizer) RequirePermission(permission Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !a.enabled {
				next.ServeHTTP(w, r)
				return
			}

			principal, ok := FromContext(r.Context())
			if !ok {
				writeSecurityError(w, http.StatusUnauthorized, "missing API key")
				return
			}
			if !principal.HasPermission(permission) {
				writeSecurityError(w, http.StatusForbidden, "principal does not have permission: "+string(permission))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Protect composes Authenticate and RequirePermission(permission) around
// handler in the correct order, for the common case of a single protected
// endpoint. Equivalent to
// authorizer.Authenticate(authorizer.RequirePermission(permission)(handler)).
func (a *Authorizer) Protect(permission Permission, handler http.HandlerFunc) http.Handler {
	return a.Authenticate(a.RequirePermission(permission)(handler))
}

func writeSecurityError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
