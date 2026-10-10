// Package postgres implements the custody repository on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/edgar/inventario/backend/internal/custody/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo implements app.AssignmentRepository on one pool.
type Repo struct{ pool *pgxpool.Pool }

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const (
	pgUnique     = "23505"
	pgForeignKey = "23503"
)

func validUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

// mapWriteErr turns constraint violations into domain errors. The unique
// index is the backstop for concurrent loans; the row lock normally makes the
// second caller see the active assignment first.
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch {
	case pgErr.Code == pgUnique && pgErr.ConstraintName == "custody_assignments_one_active_per_item":
		return domain.ErrAlreadyAssigned
	case pgErr.Code == pgForeignKey && pgErr.ConstraintName == "custody_assignments_assignee_location_id_fkey":
		return domain.FieldError("assignee_location_id", "location does not exist")
	case pgErr.Code == pgForeignKey && (pgErr.ConstraintName == "items_location_id_fkey" ||
		pgErr.ConstraintName == "custody_assignments_returned_to_location_id_fkey"):
		return domain.FieldError("return_location_id", "location does not exist")
	}
	return err
}

// lockItem locks the item row until the transaction ends and loads its state,
// including its active assignment. Concurrent loans, returns, stock movements
// and catalog edits of one item therefore serialize.
func lockItem(ctx context.Context, tx pgx.Tx, itemID string) (domain.ItemState, error) {
	var st domain.ItemState
	var kind, status string
	err := tx.QueryRow(ctx,
		`SELECT kind, status, location_id FROM items WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`,
		itemID).Scan(&kind, &status, &st.LocationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, domain.ErrItemNotFound
	}
	if err != nil {
		return st, fmt.Errorf("lock item: %w", err)
	}
	st.Kind, st.Status = domain.Kind(kind), domain.Status(status)

	var a domain.Assignment
	err = tx.QueryRow(ctx, `
		SELECT id, assignee_location_id, origin_location_id, assigned_at, due_at
		FROM custody_assignments WHERE item_id = $1::uuid AND returned_at IS NULL`, itemID).
		Scan(&a.ID, &a.AssigneeLocationID, &a.OriginLocationID, &a.AssignedAt, &a.DueAt)
	switch {
	case err == nil:
		a.ItemID = itemID
		st.Active = &a
	case !errors.Is(err, pgx.ErrNoRows):
		return st, fmt.Errorf("load active assignment: %w", err)
	}
	return st, nil
}

func loadPlace(ctx context.Context, tx pgx.Tx, id int64) (*domain.Place, error) {
	var p domain.Place
	var kind string
	err := tx.QueryRow(ctx, `SELECT id, name, kind FROM locations WHERE id = $1`, id).Scan(&p.ID, &p.Name, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load location: %w", err)
	}
	p.Kind = domain.PlaceKind(kind)
	return &p, nil
}

// Assign implements app.AssignmentRepository.
func (r *Repo) Assign(ctx context.Context, itemID, userID string, assigneeID int64, decide func(domain.ItemState) (domain.AssignOutcome, error)) (domain.Assignment, error) {
	if !validUUID(itemID) {
		return domain.Assignment{}, domain.ErrItemNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Assignment{}, fmt.Errorf("begin assign: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	st, err := lockItem(ctx, tx, itemID)
	if err != nil {
		return domain.Assignment{}, err
	}
	if assigneeID > 0 {
		if st.Assignee, err = loadPlace(ctx, tx, assigneeID); err != nil {
			return domain.Assignment{}, err
		}
	}
	out, err := decide(st)
	if err != nil {
		return domain.Assignment{}, err
	}
	a := out.Assignment
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO custody_assignments (item_id, assignee_location_id, origin_location_id, assigned_at, due_at, assigned_by, assign_note)
		VALUES ($1::uuid, $2, $3, $4, $5, $6::uuid, $7) RETURNING id`,
		itemID, a.AssigneeLocationID, a.OriginLocationID, a.AssignedAt, a.DueAt, userID, a.AssignNote).Scan(&id)
	if err != nil {
		return domain.Assignment{}, mapWriteErr(fmt.Errorf("insert assignment: %w", err))
	}
	if _, err := tx.Exec(ctx, `UPDATE items SET location_id = $2, updated_at = now() WHERE id = $1::uuid`,
		itemID, out.LocationID); err != nil {
		return domain.Assignment{}, mapWriteErr(fmt.Errorf("move item: %w", err))
	}
	res, err := getByID(ctx, tx, id)
	if err != nil {
		return domain.Assignment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Assignment{}, fmt.Errorf("commit assign: %w", err)
	}
	return res, nil
}

// Return implements app.AssignmentRepository.
func (r *Repo) Return(ctx context.Context, itemID, userID string, returnLocationID *int64, decide func(domain.ItemState) (domain.ReturnOutcome, error)) (domain.Assignment, error) {
	if !validUUID(itemID) {
		return domain.Assignment{}, domain.ErrItemNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Assignment{}, fmt.Errorf("begin return: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	st, err := lockItem(ctx, tx, itemID)
	if err != nil {
		return domain.Assignment{}, err
	}
	if returnLocationID != nil {
		if st.ReturnTo, err = loadPlace(ctx, tx, *returnLocationID); err != nil {
			return domain.Assignment{}, err
		}
	}
	out, err := decide(st)
	if err != nil {
		return domain.Assignment{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE custody_assignments
		SET returned_at = $2, returned_to_location_id = $3, returned_by = $4::uuid, return_note = $5
		WHERE id = $1`, out.AssignmentID, out.ReturnedAt, out.ReturnedTo, userID, out.ReturnNote); err != nil {
		return domain.Assignment{}, mapWriteErr(fmt.Errorf("close assignment: %w", err))
	}
	if _, err := tx.Exec(ctx, `UPDATE items SET location_id = $2, updated_at = now() WHERE id = $1::uuid`,
		itemID, out.ReturnedTo); err != nil {
		return domain.Assignment{}, mapWriteErr(fmt.Errorf("move item: %w", err))
	}
	res, err := getByID(ctx, tx, out.AssignmentID)
	if err != nil {
		return domain.Assignment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Assignment{}, fmt.Errorf("commit return: %w", err)
	}
	return res, nil
}

const selectAssignment = `
	SELECT ca.id, ca.item_id::text, i.code, i.description,
	       ca.assignee_location_id, al.name, al.kind,
	       ca.origin_location_id, ol.name,
	       ca.assigned_at, ca.due_at, ca.returned_at,
	       ca.returned_to_location_id, rl.name,
	       ca.assigned_by::text, au.name, ca.returned_by::text, ru.name,
	       ca.assign_note, ca.return_note, ca.created_at
	FROM custody_assignments ca
	JOIN items i ON i.id = ca.item_id
	JOIN locations al ON al.id = ca.assignee_location_id
	JOIN users au ON au.id = ca.assigned_by
	LEFT JOIN locations ol ON ol.id = ca.origin_location_id
	LEFT JOIN locations rl ON rl.id = ca.returned_to_location_id
	LEFT JOIN users ru ON ru.id = ca.returned_by`

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanAssignment(row pgx.Row) (domain.Assignment, error) {
	var a domain.Assignment
	var kind string
	err := row.Scan(&a.ID, &a.ItemID, &a.ItemCode, &a.ItemDescription,
		&a.AssigneeLocationID, &a.AssigneeName, &kind,
		&a.OriginLocationID, &a.OriginName,
		&a.AssignedAt, &a.DueAt, &a.ReturnedAt,
		&a.ReturnedToID, &a.ReturnedToName,
		&a.AssignedBy, &a.AssignedByName, &a.ReturnedBy, &a.ReturnedByName,
		&a.AssignNote, &a.ReturnNote, &a.CreatedAt)
	a.AssigneeKind = domain.PlaceKind(kind)
	return a, err
}

func getByID(ctx context.Context, q querier, id int64) (domain.Assignment, error) {
	a, err := scanAssignment(q.QueryRow(ctx, selectAssignment+` WHERE ca.id = $1`, id))
	if err != nil {
		return domain.Assignment{}, fmt.Errorf("read assignment: %w", err)
	}
	return a, nil
}

func collect(rows pgx.Rows) ([]domain.Assignment, error) {
	defer rows.Close()
	out := []domain.Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// History implements app.AssignmentRepository.
func (r *Repo) History(ctx context.Context, itemID string, p domain.Page) ([]domain.Assignment, int, error) {
	if !validUUID(itemID) {
		return nil, 0, domain.ErrItemNotFound
	}
	var exists bool
	if err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM items WHERE id = $1::uuid AND deleted_at IS NULL)`, itemID).Scan(&exists); err != nil {
		return nil, 0, fmt.Errorf("check item: %w", err)
	}
	if !exists {
		return nil, 0, domain.ErrItemNotFound
	}
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM custody_assignments WHERE item_id = $1::uuid`, itemID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count assignments: %w", err)
	}
	rows, err := r.pool.Query(ctx, selectAssignment+`
		WHERE ca.item_id = $1::uuid
		ORDER BY ca.assigned_at DESC, ca.id DESC
		LIMIT $2 OFFSET $3`, itemID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list assignments: %w", err)
	}
	out, err := collect(rows)
	return out, total, err
}

// Active implements app.AssignmentRepository.
func (r *Repo) Active(ctx context.Context, f domain.ActiveFilter, p domain.Page) ([]domain.Assignment, int, error) {
	conds := []string{"ca.returned_at IS NULL", "i.deleted_at IS NULL"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.AssigneeLocationID != nil {
		conds = append(conds, "ca.assignee_location_id = "+arg(*f.AssigneeLocationID))
	}
	if f.OverdueOnly {
		conds = append(conds, "ca.due_at < "+arg(f.Now))
	}
	if f.Q != "" {
		q := arg(escapeLike(f.Q))
		conds = append(conds, fmt.Sprintf(
			`(i.code ILIKE '%%' || %s || '%%' ESCAPE '\' OR i.description ILIKE '%%' || %s || '%%' ESCAPE '\')`, q, q))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM custody_assignments ca JOIN items i ON i.id = ca.item_id WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count active assignments: %w", err)
	}
	nowArg := arg(f.Now)
	limit, offset := arg(p.Limit), arg(p.Offset)
	rows, err := r.pool.Query(ctx, selectAssignment+` WHERE `+where+`
		ORDER BY (ca.due_at IS NOT NULL AND ca.due_at < `+nowArg+`) DESC, ca.assigned_at ASC, ca.id ASC
		LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list active assignments: %w", err)
	}
	out, err := collect(rows)
	return out, total, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
