// Package postgres persists project templates as atomic version-independent documents.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/projects/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

type Repo struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool} }

const cols = `id,name,description,characteristics,consumables,tools,estimated_days,created_at,updated_at`

func mapErr(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(e, &p) && p.Code == "23505" && p.ConstraintName == "project_templates_name_live_uq" {
		return domain.ErrNameTaken
	}
	return e
}
func scan(row pgx.Row, p *domain.Project) error {
	var c, m, t []byte
	e := row.Scan(&p.ID, &p.Name, &p.Description, &c, &m, &t, &p.EstimatedDays, &p.CreatedAt, &p.UpdatedAt)
	if e != nil {
		return mapErr(e)
	}
	for _, v := range []struct {
		b   []byte
		dst any
	}{{c, &p.Characteristics}, {m, &p.Consumables}, {t, &p.Tools}} {
		if e = json.Unmarshal(v.b, v.dst); e != nil {
			return e
		}
	}
	return nil
}
func encoded(p *domain.Project) ([]byte, []byte, []byte, error) {
	c, e := json.Marshal(p.Characteristics)
	if e != nil {
		return nil, nil, nil, e
	}
	m, e := json.Marshal(p.Consumables)
	if e != nil {
		return nil, nil, nil, e
	}
	t, e := json.Marshal(p.Tools)
	return c, m, t, e
}
func (r *Repo) Create(ctx context.Context, p *domain.Project) error {
	c, m, t, e := encoded(p)
	if e != nil {
		return e
	}
	tx, commit, rollback, e := db.Begin(ctx, r.pool)
	if e != nil {
		return e
	}
	defer rollback()
	e = scan(tx.QueryRow(ctx, `INSERT INTO project_templates(name,description,characteristics,consumables,tools,estimated_days)VALUES($1,$2,$3,$4,$5,$6)RETURNING `+cols, p.Name, p.Description, c, m, t, p.EstimatedDays), p)
	if e != nil {
		return e
	}
	return commit()
}
func (r *Repo) Get(ctx context.Context, id int64) (domain.Project, error) {
	var p domain.Project
	e := scan(r.pool.QueryRow(ctx, `SELECT `+cols+` FROM project_templates WHERE id=$1 AND deleted_at IS NULL`, id), &p)
	return p, e
}
func (r *Repo) Update(ctx context.Context, p *domain.Project) error {
	c, m, t, e := encoded(p)
	if e != nil {
		return e
	}
	tx, commit, rollback, e := db.Begin(ctx, r.pool)
	if e != nil {
		return e
	}
	defer rollback()
	e = scan(tx.QueryRow(ctx, `UPDATE project_templates SET name=$2,description=$3,characteristics=$4,consumables=$5,tools=$6,estimated_days=$7,updated_at=now() WHERE id=$1 AND deleted_at IS NULL RETURNING `+cols, p.ID, p.Name, p.Description, c, m, t, p.EstimatedDays), p)
	if e != nil {
		return e
	}
	return commit()
}
func (r *Repo) Delete(ctx context.Context, id int64) error {
	tag, e := r.pool.Exec(ctx, `UPDATE project_templates SET deleted_at=now(),updated_at=now() WHERE id=$1 AND deleted_at IS NULL`, id)
	if e == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return e
}

var escape = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (r *Repo) List(ctx context.Context, f domain.Filter) ([]domain.Project, int, error) {
	pattern := "%" + escape.Replace(f.Q) + "%"
	condition := `deleted_at IS NULL AND (name ILIKE $1 OR description ILIKE $1)`
	var count int
	if e := r.pool.QueryRow(ctx, `SELECT count(*) FROM project_templates WHERE `+condition, pattern).Scan(&count); e != nil {
		return nil, 0, e
	}
	rows, e := r.pool.Query(ctx, fmt.Sprintf(`SELECT %s FROM project_templates WHERE %s ORDER BY lower(name),id LIMIT $2 OFFSET $3`, cols, condition), pattern, f.Limit, f.Offset)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []domain.Project{}
	for rows.Next() {
		var p domain.Project
		if e = scan(rows, &p); e != nil {
			return nil, 0, e
		}
		out = append(out, p)
	}
	return out, count, rows.Err()
}
