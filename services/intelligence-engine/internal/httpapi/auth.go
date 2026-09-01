package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/atlas/intelligence-engine/internal/security"
)

// AuthAPI exposes read-only information about the caller's OWN already-
// resolved identity. It never accepts new input beyond the existing
// X-Atlas-Api-Key header, never returns the key itself, and never returns
// anything beyond what internal/security already resolved that key to
// (Principal.Name/Role) plus the permission set that role grants -- both
// already public knowledge via security's role->permission table, not new
// sensitive material.
type AuthAPI struct{}

func NewAuthAPI() *AuthAPI {
	return &AuthAPI{}
}

// The five permissions this system knows about (see security/model.go's own
// comment: "no permission beyond the five above has any handler to attach
// to"). Listed here, not exported from internal/security, to avoid growing
// that package's surface for a single read-only convenience loop.
var allPermissions = []security.Permission{
	security.PermissionView,
	security.PermissionCreatePlan,
	security.PermissionApprovePlan,
	security.PermissionExecute,
	security.PermissionReadAudit,
}

type identityResponse struct {
	// SecurityEnabled distinguishes "no principal because auth is off" from
	// "no principal because rejected" -- the latter never reaches this
	// handler at all (see HandleGetMe), so false here always and only means
	// ATLAS_SECURITY_ENABLED=false.
	SecurityEnabled bool     `json:"securityEnabled"`
	Name            string   `json:"name,omitempty"`
	Role            string   `json:"role,omitempty"`
	Permissions     []string `json:"permissions,omitempty"`
}

// HandleGetMe handles GET /api/v1/auth/me. Registered behind
// authorizer.Protect(security.PermissionView, ...) like every other read
// endpoint: every real role holds VIEW, so this never itself restricts who
// can see their own identity, while still returning a real 401 for a
// missing/invalid key when security is enabled.
//
// When ATLAS_SECURITY_ENABLED=false, Protect is a pure pass-through (see
// Authorizer.Authenticate) and attaches no Principal at all -- that is the
// ONLY way this handler can run without one, since Protect would otherwise
// have already rejected the request with 401 before reaching here. So
// FromContext's ok result alone is sufficient to tell the two states apart.
func (a *AuthAPI) HandleGetMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	principal, ok := security.FromContext(r.Context())

	resp := identityResponse{SecurityEnabled: ok}
	if ok {
		resp.Name = principal.Name
		resp.Role = string(principal.Role)
		resp.Permissions = make([]string, 0, len(allPermissions))
		for _, perm := range allPermissions {
			if principal.HasPermission(perm) {
				resp.Permissions = append(resp.Permissions, string(perm))
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
