package domain

// Roles returns all supported role identities in display order.
func Roles() []Role {
	return []Role{RoleAdmin, RoleWorkshopChief, RoleSupervisor, RoleInventoryChief, RoleWorker, RoleOperator, RoleViewer}
}

// Permissions returns the supported capability catalog. New business modules extend this catalog.
func Permissions() []Permission {
	return []Permission{PermRead, PermWrite, PermWriteItems, PermManageCatalogs, PermManageUsers, PermManageRoles}
}

// ValidateRolePermissions prevents unknown capabilities and administrator lockout.
func ValidateRolePermissions(role Role, permissions []Permission) error {
	if !role.Valid() || permissions == nil {
		return ErrInvalidInput
	}
	known := map[Permission]bool{}
	for _, p := range Permissions() {
		known[p] = true
	}
	seen := map[Permission]bool{}
	for _, p := range permissions {
		if !known[p] || seen[p] {
			return ErrInvalidInput
		}
		seen[p] = true
	}
	if role == RoleAdmin && (!seen[PermManageRoles] || !seen[PermManageUsers]) {
		return ErrInvalidInput
	}
	return nil
}
