package memory

import (
	"context"
	"github.com/edgar/inventario/backend/internal/identity/domain"
)

// RolePermissions returns a detached copy of a role's grant set.
func (s *Store) RolePermissions(_ context.Context, role domain.Role) ([]domain.Permission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ps, ok := s.permissions[role]
	if !ok {
		return nil, domain.ErrInvalidInput
	}
	return append([]domain.Permission{}, ps...), nil
}

// SetRolePermissions replaces grants under the store lock.
func (s *Store) SetRolePermissions(_ context.Context, role domain.Role, ps []domain.Permission) error {
	if err := domain.ValidateRolePermissions(role, ps); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.permissions[role] = append([]domain.Permission{}, ps...)
	return nil
}
