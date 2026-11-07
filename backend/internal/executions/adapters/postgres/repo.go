// Package postgres persists manufacturing execution snapshots and assignments.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

type rowQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

const selectExecution = `SELECT e.id,e.project_snapshot,e.units,e.estimated_days,e.status,e.principal_user_id::text,e.created_at,e.updated_at,
ARRAY(SELECT customer_id FROM manufacturing_execution_customers c WHERE c.execution_id=e.id ORDER BY customer_id)::BIGINT[],
ARRAY(SELECT user_id::text FROM manufacturing_execution_workers w WHERE w.execution_id=e.id ORDER BY user_id)::TEXT[]
FROM manufacturing_executions e`

func scanExecution(row pgx.Row) (domain.Execution, error) {
	var e domain.Execution
	var snapshot []byte
	var status string
	err := row.Scan(&e.ID, &snapshot, &e.Units, &e.EstimatedDays, &status, &e.PrincipalUserID, &e.CreatedAt, &e.UpdatedAt, &e.CustomerIDs, &e.WorkerIDs)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Execution{}, domain.ErrNotFound
		}
		return domain.Execution{}, err
	}
	if err := json.Unmarshal(snapshot, &e.ProjectSnapshot); err != nil {
		return domain.Execution{}, err
	}
	e.Status = domain.Status(status)
	return e, nil
}

func (r *Repo) Create(ctx context.Context, e domain.Execution) (domain.Execution, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rollback()
	customerRows, err := tx.Query(ctx, `SELECT id FROM customers WHERE id = ANY($1::BIGINT[]) AND active AND deleted_at IS NULL ORDER BY id FOR SHARE`, e.CustomerIDs)
	if err != nil {
		return domain.Execution{}, err
	}
	customerCount := 0
	for customerRows.Next() {
		var ignored int64
		if err = customerRows.Scan(&ignored); err != nil {
			customerRows.Close()
			return domain.Execution{}, err
		}
		customerCount++
	}
	err = customerRows.Err()
	customerRows.Close()
	if err != nil {
		return domain.Execution{}, err
	}
	if customerCount != len(e.CustomerIDs) {
		return domain.Execution{}, domain.ErrInactiveAssignment
	}
	users := append([]string{e.PrincipalUserID}, e.WorkerIDs...)
	userRows, err := tx.Query(ctx, `SELECT id FROM users WHERE id::text = ANY($1::TEXT[]) AND active ORDER BY id FOR SHARE`, users)
	if err != nil {
		return domain.Execution{}, err
	}
	userCount := 0
	for userRows.Next() {
		var ignored string
		if err = userRows.Scan(&ignored); err != nil {
			userRows.Close()
			return domain.Execution{}, err
		}
		userCount++
	}
	err = userRows.Err()
	userRows.Close()
	if err != nil {
		return domain.Execution{}, err
	}
	if userCount != len(users) {
		return domain.Execution{}, domain.ErrInactiveAssignment
	}
	snapshot, err := json.Marshal(e.ProjectSnapshot)
	if err != nil {
		return domain.Execution{}, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO manufacturing_executions(project_template_id,project_snapshot,units,estimated_days,status,principal_user_id)
VALUES($1,$2,$3,$4,$5,$6) RETURNING id,created_at,updated_at`, e.ProjectID, snapshot, e.Units, e.EstimatedDays, string(e.Status), e.PrincipalUserID).Scan(&e.ID, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return domain.Execution{}, err
	}
	for _, id := range e.CustomerIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO manufacturing_execution_customers(execution_id,customer_id) VALUES($1,$2)`, e.ID, id); err != nil {
			return domain.Execution{}, err
		}
	}
	for _, id := range e.WorkerIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO manufacturing_execution_workers(execution_id,user_id) VALUES($1,$2)`, e.ID, id); err != nil {
			return domain.Execution{}, err
		}
	}
	if err = commit(); err != nil {
		return domain.Execution{}, err
	}
	return e.Clone(), nil
}

func (r *Repo) Get(ctx context.Context, id int64) (domain.Execution, error) {
	return scanExecution(r.pool.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, id))
}

var escape = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (r *Repo) List(ctx context.Context, f domain.Filter) ([]domain.Execution, int, error) {
	status := any(nil)
	if f.Status != nil {
		status = string(*f.Status)
	}
	pattern := ""
	if f.Q != "" {
		pattern = "%" + escape.Replace(f.Q) + "%"
	}
	where := `($1::TEXT IS NULL OR status=$1) AND ($2='' OR project_snapshot->>'project_name' ILIKE $2 ESCAPE '\')`
	var count int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM manufacturing_executions WHERE `+where, status, pattern).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, selectExecution+` WHERE `+where+` ORDER BY e.created_at DESC,e.id DESC LIMIT $3 OFFSET $4`, status, pattern, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []domain.Execution{}
	for rows.Next() {
		e, scanErr := scanExecution(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		out = append(out, e)
	}
	return out, count, rows.Err()
}

func (r *Repo) Transition(ctx context.Context, id int64, from, to domain.Status) (domain.Execution, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rollback()
	result, err := tx.Exec(ctx, `UPDATE manufacturing_executions SET status=$3,updated_at=now() WHERE id=$1 AND status=$2`, id, string(from), string(to))
	if err != nil {
		return domain.Execution{}, err
	}
	if result.RowsAffected() == 0 {
		var current string
		err = tx.QueryRow(ctx, `SELECT status FROM manufacturing_executions WHERE id=$1`, id).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Execution{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.Execution{}, err
		}
		return domain.Execution{}, domain.ErrInvalidState
	}
	e, err := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, id))
	if err != nil {
		return domain.Execution{}, err
	}
	if err = commit(); err != nil {
		return domain.Execution{}, err
	}
	return e, nil
}

func (r *Repo) ListAssignableUsers(ctx context.Context) ([]domain.AssignableUser, error) {
	rows, err := r.pool.Query(ctx, `SELECT id::text,name FROM users WHERE active ORDER BY lower(name),id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.AssignableUser{}
	for rows.Next() {
		var user domain.AssignableUser
		if err := rows.Scan(&user.ID, &user.Name); err != nil {
			return nil, err
		}
		out = append(out, user)
	}
	return out, rows.Err()
}
