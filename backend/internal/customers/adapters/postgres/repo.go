// Package postgres implements the customer repository on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/edgar/inventario/backend/internal/customers/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	pgUnique   = "23505"
	nameLiveUQ = "customers_name_live_uq"
)

const columns = `id, name, identification, contact, notes, active, created_at, updated_at`

// Repo implements the customer repository port.
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

func scan(row pgx.Row, s *domain.Customer) error {
	return row.Scan(&s.ID, &s.Name, &s.Identification, &s.Contact, &s.Notes, &s.Active, &s.CreatedAt, &s.UpdatedAt)
}

// Create inserts s and fills its ID and timestamps.
func (r *Repo) Create(ctx context.Context, s *domain.Customer) error {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO customers (name, identification, contact, notes, active)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+columns,
		s.Name, s.Identification, s.Contact, s.Notes, s.Active)
	return mapErr(scan(row, s))
}

// Get returns a live customer.
func (r *Repo) Get(ctx context.Context, id int64) (domain.Customer, error) {
	var s domain.Customer
	err := scan(r.pool.QueryRow(ctx,
		`SELECT `+columns+` FROM customers WHERE id = $1 AND deleted_at IS NULL`, id), &s)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Customer{}, domain.ErrNotFound
	}
	return s, err
}

// Update stores the editable fields and refreshes updated_at.
func (r *Repo) Update(ctx context.Context, s *domain.Customer) error {
	err := scan(r.pool.QueryRow(ctx,
		`UPDATE customers
		    SET name = $2, identification = $3, contact = $4, notes = $5,
		        active = $6, updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL
		  RETURNING `+columns,
		s.ID, s.Name, s.Identification, s.Contact, s.Notes, s.Active), s)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return mapErr(err)
}

// Delete soft-deletes a live customer.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE customers SET deleted_at = now(), updated_at = now()
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

// List returns a page of live customers ordered by name, and the total count.
func (r *Repo) List(ctx context.Context, f domain.Filter) ([]domain.Customer, int, error) {
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	if f.Q != "" {
		args = append(args, "%"+escapeLike.Replace(f.Q)+"%")
		n := len(args)
		where = append(where, fmt.Sprintf(
			"(name ILIKE $%[1]d OR identification ILIKE $%[1]d OR contact ILIKE $%[1]d)", n))
	}
	if f.Active != nil {
		args = append(args, *f.Active)
		where = append(where, fmt.Sprintf("active = $%d", len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM customers WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT `+columns+` FROM customers WHERE %s ORDER BY lower(name), id LIMIT $%d OFFSET $%d`,
			cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.Customer{}
	for rows.Next() {
		var s domain.Customer
		if err := scan(rows, &s); err != nil {
			return nil, 0, err
		}
		out = append(out, s)
	}
	return out, total, rows.Err()
}
