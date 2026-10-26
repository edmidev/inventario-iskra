// Package postgres implements the supplier repository on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/edgar/inventario/backend/internal/suppliers/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pgUnique   = "23505"
	nameLiveUQ = "suppliers_name_live_uq"
)

const columns = `id, name, contact_name, email, phone, notes, active, created_at, updated_at`

// Repo implements the supplier repository port.
type Repo struct{ pool *pgxpool.Pool }

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func mapErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUnique && pgErr.ConstraintName == nameLiveUQ {
		return domain.ErrNameTaken
	}
	return err
}

func scan(row pgx.Row, s *domain.Supplier) error {
	return row.Scan(&s.ID, &s.Name, &s.ContactName, &s.Email, &s.Phone, &s.Notes, &s.Active, &s.CreatedAt, &s.UpdatedAt)
}

// Create inserts s and fills its ID and timestamps.
func (r *Repo) Create(ctx context.Context, s *domain.Supplier) error {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO suppliers (name, contact_name, email, phone, notes, active)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+columns,
		s.Name, s.ContactName, s.Email, s.Phone, s.Notes, s.Active)
	return mapErr(scan(row, s))
}

// Get returns a live supplier.
func (r *Repo) Get(ctx context.Context, id int64) (domain.Supplier, error) {
	var s domain.Supplier
	err := scan(r.pool.QueryRow(ctx,
		`SELECT `+columns+` FROM suppliers WHERE id = $1 AND deleted_at IS NULL`, id), &s)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Supplier{}, domain.ErrNotFound
	}
	return s, err
}

// Update stores the editable fields and refreshes updated_at.
func (r *Repo) Update(ctx context.Context, s *domain.Supplier) error {
	err := scan(r.pool.QueryRow(ctx,
		`UPDATE suppliers
		    SET name = $2, contact_name = $3, email = $4, phone = $5, notes = $6,
		        active = $7, updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL
		  RETURNING `+columns,
		s.ID, s.Name, s.ContactName, s.Email, s.Phone, s.Notes, s.Active), s)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return mapErr(err)
}

// Delete soft-deletes a live supplier.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE suppliers SET deleted_at = now(), updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// escapeLike makes user text literal inside an ILIKE pattern.
var escapeLike = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// List returns a page of live suppliers ordered by name, and the total count.
func (r *Repo) List(ctx context.Context, f domain.Filter) ([]domain.Supplier, int, error) {
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	if f.Q != "" {
		args = append(args, "%"+escapeLike.Replace(f.Q)+"%")
		n := len(args)
		where = append(where, fmt.Sprintf(
			"(name ILIKE $%[1]d OR contact_name ILIKE $%[1]d OR email ILIKE $%[1]d)", n))
	}
	if f.Active != nil {
		args = append(args, *f.Active)
		where = append(where, fmt.Sprintf("active = $%d", len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM suppliers WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT `+columns+` FROM suppliers WHERE %s ORDER BY lower(name), id LIMIT $%d OFFSET $%d`,
			cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.Supplier{}
	for rows.Next() {
		var s domain.Supplier
		if err := scan(rows, &s); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}
