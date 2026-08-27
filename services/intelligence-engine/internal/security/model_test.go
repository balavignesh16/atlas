package security

import "testing"

func TestHasPermission_RoleMapping(t *testing.T) {
	cases := []struct {
		role       Role
		permission Permission
		want       bool
	}{
		{RoleViewer, PermissionView, true},
		{RoleViewer, PermissionReadAudit, true},
		{RoleViewer, PermissionCreatePlan, false},
		{RoleViewer, PermissionApprovePlan, false},
		{RoleViewer, PermissionExecute, false},

		{RoleOperator, PermissionView, true},
		{RoleOperator, PermissionCreatePlan, true},
		{RoleOperator, PermissionApprovePlan, false},
		{RoleOperator, PermissionExecute, false},
		{RoleOperator, PermissionReadAudit, false},

		{RoleApprover, PermissionView, true},
		{RoleApprover, PermissionApprovePlan, true},
		{RoleApprover, PermissionReadAudit, true},
		{RoleApprover, PermissionCreatePlan, false},
		{RoleApprover, PermissionExecute, false},

		{RoleExecutor, PermissionView, true},
		{RoleExecutor, PermissionExecute, true},
		{RoleExecutor, PermissionReadAudit, true},
		{RoleExecutor, PermissionCreatePlan, false},
		{RoleExecutor, PermissionApprovePlan, false},

		{RoleAdmin, PermissionView, true},
		{RoleAdmin, PermissionCreatePlan, true},
		{RoleAdmin, PermissionApprovePlan, true},
		{RoleAdmin, PermissionExecute, true},
		{RoleAdmin, PermissionReadAudit, true},
	}

	for _, c := range cases {
		t.Run(string(c.role)+"/"+string(c.permission), func(t *testing.T) {
			if got := HasPermission(c.role, c.permission); got != c.want {
				t.Errorf("HasPermission(%s, %s) = %v, want %v", c.role, c.permission, got, c.want)
			}
		})
	}
}

func TestHasPermission_UnrecognizedRoleFailsClosed(t *testing.T) {
	if HasPermission(Role("NOT_A_REAL_ROLE"), PermissionView) {
		t.Fatal("expected an unrecognized role to have zero permissions, fail closed")
	}
}

func TestIsValidRole(t *testing.T) {
	for _, r := range []Role{RoleViewer, RoleOperator, RoleApprover, RoleExecutor, RoleAdmin} {
		if !IsValidRole(r) {
			t.Errorf("expected %s to be a valid role", r)
		}
	}
	if IsValidRole(Role("BOGUS")) {
		t.Fatal("expected an unrecognized role string to be invalid")
	}
}

func TestPrincipal_HasPermission(t *testing.T) {
	p := Principal{Name: "alice", Role: RoleOperator}
	if !p.HasPermission(PermissionCreatePlan) {
		t.Fatal("expected an OPERATOR principal to have CREATE_PLAN")
	}
	if p.HasPermission(PermissionExecute) {
		t.Fatal("expected an OPERATOR principal to NOT have EXECUTE")
	}
}
