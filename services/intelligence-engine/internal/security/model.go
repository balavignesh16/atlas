// Package security implements M2.9's authentication/authorization boundary:
// a static API key resolves to a named Principal with a Role, and handlers
// declare the Permission they require rather than checking roles directly.
// This package owns the principal -> role -> permission mapping so that
// mapping never gets duplicated or drifted across handlers.
package security

// Permission is a single capability a Principal may or may not hold.
// Handlers declare the Permission they require; they never reference a
// Role directly (see RequirePermission in middleware.go).
type Permission string

const (
	PermissionView        Permission = "VIEW"
	PermissionCreatePlan  Permission = "CREATE_PLAN"
	PermissionApprovePlan Permission = "APPROVE_PLAN"
	PermissionExecute     Permission = "EXECUTE"
	PermissionReadAudit   Permission = "READ_AUDIT"
)

// Role is a named bundle of permissions. Kept intentionally small and
// matched to the actual API surface (see docs/m29_verification_report.md's
// "Authorization / RBAC model" section for the investigation this maps to):
// there is no endpoint today that would justify a role finer-grained than
// these five, and no permission beyond the five above has any handler to
// attach to.
type Role string

const (
	RoleViewer   Role = "VIEWER"
	RoleOperator Role = "OPERATOR"
	RoleApprover Role = "APPROVER"
	RoleExecutor Role = "EXECUTOR"
	RoleAdmin    Role = "ADMIN"
)

var rolePermissions = map[Role]map[Permission]bool{
	RoleViewer: {
		PermissionView:      true,
		PermissionReadAudit: true,
	},
	RoleOperator: {
		PermissionView:       true,
		PermissionCreatePlan: true,
	},
	RoleApprover: {
		PermissionView:        true,
		PermissionApprovePlan: true,
		PermissionReadAudit:   true,
	},
	RoleExecutor: {
		PermissionView:      true,
		PermissionExecute:   true,
		PermissionReadAudit: true,
	},
	RoleAdmin: {
		PermissionView:        true,
		PermissionCreatePlan:  true,
		PermissionApprovePlan: true,
		PermissionExecute:     true,
		PermissionReadAudit:   true,
	},
}

// IsValidRole reports whether role is one of the roles this system knows
// about. Used to reject malformed API-key configuration at startup rather
// than silently granting an unrecognized role zero permissions.
func IsValidRole(role Role) bool {
	_, ok := rolePermissions[role]
	return ok
}

// HasPermission reports whether role grants permission. An unrecognized
// role has no permissions -- fails closed, never open.
func HasPermission(role Role, permission Permission) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[permission]
}

// Principal is the authenticated identity attached to a request's context
// once its API key has been validated. Name is a stable, human-readable
// identity (e.g. "alice"), never the key itself -- Name is what gets
// recorded into audit fields like ApprovalMetadata.ApprovedBy and
// ExecutionRecord.Approver; the key is never logged or echoed back.
type Principal struct {
	Name string
	Role Role
}

// HasPermission reports whether this Principal's role grants permission.
func (p Principal) HasPermission(permission Permission) bool {
	return HasPermission(p.Role, permission)
}
