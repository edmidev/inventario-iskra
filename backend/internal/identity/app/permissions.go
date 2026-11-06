package app

import (
	"context"

	"github.com/edgar/inventario/backend/internal/identity/domain"
)

// RolePermissions is the persisted configuration for a role.
type RolePermissions struct {
	Role        domain.Role         `json:"role"`
	Permissions []domain.Permission `json:"permissions"`
}

// ListRoles returns effective configured grants for all seeded roles.
func (s *Service) ListRoles(ctx context.Context) ([]RolePermissions, error) {
	out := make([]RolePermissions, 0)
	for _, role := range domain.Roles() {
		ps, err := s.users.RolePermissions(ctx, role)
		if err != nil {
			return nil, err
		}
		out = append(out, RolePermissions{role, ps})
	}
	return out, nil
}

// SetRolePermissions replaces the complete validated grant set atomically.
func (s *Service) SetRolePermissions(ctx context.Context, role domain.Role, ps []domain.Permission) (RolePermissions, error) {
	if err := domain.ValidateRolePermissions(role, ps); err != nil {
		return RolePermissions{}, err
	}
	if err := s.users.SetRolePermissions(ctx, role, ps); err != nil {
		return RolePermissions{}, err
	}
	return RolePermissions{role, ps}, nil
}
func (s *Service) withPermissions(ctx context.Context, u domain.User) (domain.User, error) {
	ps, err := s.users.RolePermissions(ctx, u.Role)
	if err != nil {
		return domain.User{}, err
	}
	u.Permissions = ps
	return u, nil
}

// VerifyAccessContext resolves active identity and effective permissions on every request,
// ignoring stale role claims in a valid JWT.
func (s *Service) VerifyAccessContext(ctx context.Context, token string) (AccessClaims, error) {
	claims, err := s.issuer.ParseAccess(token, s.now())
	if err != nil {
		return AccessClaims{}, err
	}
	u, err := s.activeUser(ctx, claims.UserID)
	if err != nil {
		return AccessClaims{}, err
	}
	return AccessClaims{UserID: u.ID, Role: u.Role, Permissions: u.Permissions}, nil
}
