package postgres

import (
	"context"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
)

// RolePermissions reads the persisted set, including an empty set for an unconfigured role.
func (r *Repo) RolePermissions(ctx context.Context, role domain.Role) ([]domain.Permission, error) {
	if !role.Valid() {
		return nil, domain.ErrInvalidInput
	}
	var keys []string
	err := r.pool.QueryRow(ctx, `SELECT permissions FROM identity_roles WHERE role=$1`, string(role)).Scan(&keys)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Permission, len(keys))
	for i, k := range keys {
		out[i] = domain.Permission(k)
	}
	return out, nil
}

// SetRolePermissions updates one row atomically; concurrent replacements never mix sets.
func (r *Repo) SetRolePermissions(ctx context.Context, role domain.Role, ps []domain.Permission) error {
	if err := domain.ValidateRolePermissions(role, ps); err != nil {
		return err
	}
	keys := make([]string, len(ps))
	for i, p := range ps {
		keys[i] = string(p)
	}
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return err
	}
	defer rollback()
	tag, err := tx.Exec(ctx, `UPDATE identity_roles SET permissions=$2 WHERE role=$1`, string(role), keys)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrInvalidInput
	}
	return commit()
}
