// Package postgres implements the maintenance repository on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/edgar/inventario/backend/internal/maintenance/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo implements app.Repository on one pool.
type Repo struct{ pool *pgxpool.Pool }

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const pgUnique = "23505"

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

// mapWriteErr turns the open-failure unique index into a domain error. The
// item row lock normally makes the second caller see the open record first;
// the index is the backstop.
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUnique && pgErr.ConstraintName == "maintenance_records_one_open_failure_per_item" {
		return domain.ErrOpenFailureExists
	}
	return err
}

const selectRecord = `
	SELECT m.id, m.item_id::text, i.code, i.description, i.status, m.kind, m.status,
	       m.reported_at, m.description, m.destination, m.cost::float8, m.next_due_at, m.closed_at, m.resolution_note,
	       m.reported_by::text, ru.name, m.closed_by::text, cu.name, m.created_at, m.updated_at
	FROM maintenance_records m
	JOIN items i ON i.id = m.item_id
	JOIN users ru ON ru.id = m.reported_by
	LEFT JOIN users cu ON cu.id = m.closed_by`

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanRecord(row pgx.Row) (domain.Record, error) {
	var r domain.Record
	var itemStatus, kind, status string
	err := row.Scan(&r.ID, &r.ItemID, &r.ItemCode, &r.ItemDescription, &itemStatus, &kind, &status,
		&r.ReportedAt, &r.Description, &r.Destination, &r.Cost, &r.NextDueAt, &r.ClosedAt, &r.ResolutionNote,
		&r.ReportedBy, &r.ReportedByName, &r.ClosedBy, &r.ClosedByName, &r.CreatedAt, &r.UpdatedAt)
	r.ItemStatus, r.Kind, r.Status = domain.ItemStatus(itemStatus), domain.Kind(kind), domain.Status(status)
	return r, err
}

func getByID(ctx context.Context, q querier, id int64) (domain.Record, error) {
	r, err := scanRecord(q.QueryRow(ctx, selectRecord+` WHERE m.id = $1`, id))
	if err != nil {
		return domain.Record{}, fmt.Errorf("read maintenance record: %w", err)
	}
	return r, nil
}

func collect(rows pgx.Rows) ([]domain.Record, error) {
	defer rows.Close()
	out := []domain.Record{}
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan maintenance record: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// lockItem locks the item row until the transaction ends and loads its
// state, including its open corrective record. Concurrent maintenance writes,
// loans, stock movements and catalog edits of one item therefore serialize.
func lockItem(ctx context.Context, tx pgx.Tx, itemID string) (domain.ItemState, error) {
	var st domain.ItemState
	var kind, status string
	err := tx.QueryRow(ctx, `
		SELECT kind, status, maintenance_interval_days, last_maintenance_at
		FROM items WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`, itemID).
		Scan(&kind, &status, &st.IntervalDays, &st.LastMaintenanceAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, domain.ErrItemNotFound
	}
	if err != nil {
		return st, fmt.Errorf("lock item: %w", err)
	}
	st.Kind, st.Status = domain.ItemKind(kind), domain.ItemStatus(status)

	var openID int64
	err = tx.QueryRow(ctx,
		`SELECT id FROM maintenance_records WHERE item_id = $1::uuid AND kind = 'corrective' AND status = 'open'`, itemID).Scan(&openID)
	switch {
	case err == nil:
		open, err := getByID(ctx, tx, openID)
		if err != nil {
			return st, err
		}
		st.Open = &open
	case !errors.Is(err, pgx.ErrNoRows):
		return st, fmt.Errorf("load open failure: %w", err)
	}
	return st, nil
}

// lockByRecord resolves the record's item, locks it, and returns the item
// state with the (now stable) record. A record whose item is soft-deleted is
// reported as not found.
func lockByRecord(ctx context.Context, tx pgx.Tx, recordID int64) (domain.ItemState, domain.Record, error) {
	var itemID string
	err := tx.QueryRow(ctx, `SELECT item_id::text FROM maintenance_records WHERE id = $1`, recordID).Scan(&itemID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ItemState{}, domain.Record{}, domain.ErrRecordNotFound
	}
	if err != nil {
		return domain.ItemState{}, domain.Record{}, fmt.Errorf("find record: %w", err)
	}
	st, err := lockItem(ctx, tx, itemID)
	if errors.Is(err, domain.ErrItemNotFound) {
		return st, domain.Record{}, domain.ErrRecordNotFound
	}
	if err != nil {
		return st, domain.Record{}, err
	}
	rec, err := getByID(ctx, tx, recordID)
	return st, rec, err
}

// Report implements app.Repository.
func (r *Repo) Report(ctx context.Context, itemID, userID string, decide func(domain.ItemState) (domain.ReportOutcome, error)) (domain.Record, error) {
	if !validUUID(itemID) {
		return domain.Record{}, domain.ErrItemNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Record{}, fmt.Errorf("begin report: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	st, err := lockItem(ctx, tx, itemID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(st)
	if err != nil {
		return domain.Record{}, err
	}
	rec := out.Record
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO maintenance_records (item_id, kind, status, reported_at, description, destination, reported_by)
		VALUES ($1::uuid, 'corrective', 'open', $2, $3, $4, $5::uuid) RETURNING id`,
		itemID, rec.ReportedAt, rec.Description, rec.Destination, userID).Scan(&id)
	if err != nil {
		return domain.Record{}, mapWriteErr(fmt.Errorf("insert failure report: %w", err))
	}
	if _, err := tx.Exec(ctx, `UPDATE items SET status = $2, updated_at = now() WHERE id = $1::uuid`,
		itemID, string(out.ItemStatus)); err != nil {
		return domain.Record{}, fmt.Errorf("update item status: %w", err)
	}
	return commitWithRecord(ctx, tx, id)
}

func commitWithRecord(ctx context.Context, tx pgx.Tx, id int64) (domain.Record, error) {
	res, err := getByID(ctx, tx, id)
	if err != nil {
		return domain.Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Record{}, fmt.Errorf("commit maintenance: %w", err)
	}
	return res, nil
}

// RegisterPreventive implements app.Repository.
func (r *Repo) RegisterPreventive(ctx context.Context, itemID, userID string, decide func(domain.ItemState) (domain.PreventiveOutcome, error)) (domain.Record, error) {
	if !validUUID(itemID) {
		return domain.Record{}, domain.ErrItemNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Record{}, fmt.Errorf("begin preventive: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	st, err := lockItem(ctx, tx, itemID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(st)
	if err != nil {
		return domain.Record{}, err
	}
	rec := out.Record
	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO maintenance_records (item_id, kind, status, reported_at, description, cost, next_due_at, closed_at, reported_by, closed_by)
		VALUES ($1::uuid, 'preventive', 'closed', $2, $3, $4, $5, $2, $6::uuid, $6::uuid) RETURNING id`,
		itemID, rec.ReportedAt, rec.Description, rec.Cost, rec.NextDueAt, userID).Scan(&id)
	if err != nil {
		return domain.Record{}, fmt.Errorf("insert preventive record: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE items SET last_maintenance_at = $2, updated_at = now() WHERE id = $1::uuid`,
		itemID, out.LastMaintenanceAt); err != nil {
		return domain.Record{}, fmt.Errorf("update last maintenance: %w", err)
	}
	return commitWithRecord(ctx, tx, id)
}

// Update implements app.Repository.
func (r *Repo) Update(ctx context.Context, recordID int64, decide func(domain.ItemState, domain.Record) (domain.UpdateOutcome, error)) (domain.Record, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Record{}, fmt.Errorf("begin update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	st, rec, err := lockByRecord(ctx, tx, recordID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(st, rec)
	if err != nil {
		return domain.Record{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE maintenance_records SET description = $2, destination = $3, updated_at = now() WHERE id = $1`,
		recordID, out.Record.Description, out.Record.Destination); err != nil {
		return domain.Record{}, fmt.Errorf("update maintenance record: %w", err)
	}
	if out.ItemStatus != "" {
		if _, err := tx.Exec(ctx, `UPDATE items SET status = $2, updated_at = now() WHERE id = $1::uuid`,
			rec.ItemID, string(out.ItemStatus)); err != nil {
			return domain.Record{}, fmt.Errorf("update item status: %w", err)
		}
	}
	return commitWithRecord(ctx, tx, recordID)
}

// Close implements app.Repository.
func (r *Repo) Close(ctx context.Context, recordID int64, userID string, decide func(domain.ItemState, domain.Record) (domain.CloseOutcome, error)) (domain.Record, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Record{}, fmt.Errorf("begin close: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	st, rec, err := lockByRecord(ctx, tx, recordID)
	if err != nil {
		return domain.Record{}, err
	}
	out, err := decide(st, rec)
	if err != nil {
		return domain.Record{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE maintenance_records
		SET status = 'closed', closed_at = $2, closed_by = $3::uuid, cost = $4, resolution_note = $5, updated_at = now()
		WHERE id = $1`, recordID, out.Record.ClosedAt, userID, out.Record.Cost, out.Record.ResolutionNote); err != nil {
		return domain.Record{}, fmt.Errorf("close maintenance record: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE items SET status = $2, last_maintenance_at = COALESCE($3, last_maintenance_at), updated_at = now()
		WHERE id = $1::uuid`, rec.ItemID, string(out.ItemStatus), out.LastMaintenanceAt); err != nil {
		return domain.Record{}, fmt.Errorf("update item: %w", err)
	}
	return commitWithRecord(ctx, tx, recordID)
}

// History implements app.Repository.
func (r *Repo) History(ctx context.Context, itemID string, p domain.Page) ([]domain.Record, int, error) {
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
		`SELECT count(*) FROM maintenance_records WHERE item_id = $1::uuid`, itemID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count maintenance records: %w", err)
	}
	rows, err := r.pool.Query(ctx, selectRecord+`
		WHERE m.item_id = $1::uuid
		ORDER BY m.reported_at DESC, m.id DESC
		LIMIT $2 OFFSET $3`, itemID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list maintenance history: %w", err)
	}
	out, err := collect(rows)
	return out, total, err
}

// List implements app.Repository.
func (r *Repo) List(ctx context.Context, f domain.ListFilter, p domain.Page) ([]domain.Record, int, error) {
	conds := []string{"i.deleted_at IS NULL"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.Status != "" {
		conds = append(conds, "m.status = "+arg(string(f.Status)))
	}
	if f.Kind != "" {
		conds = append(conds, "m.kind = "+arg(string(f.Kind)))
	}
	if f.ItemID != "" {
		conds = append(conds, "m.item_id = "+arg(f.ItemID)+"::uuid")
	}
	if f.Destination != "" {
		conds = append(conds, "lower(m.destination) = lower("+arg(f.Destination)+")")
	}
	if f.Q != "" {
		q := arg(escapeLike(f.Q))
		conds = append(conds, fmt.Sprintf(
			`(i.code ILIKE '%%' || %s || '%%' ESCAPE '\' OR i.description ILIKE '%%' || %s || '%%' ESCAPE '\' OR m.description ILIKE '%%' || %s || '%%' ESCAPE '\')`, q, q, q))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM maintenance_records m JOIN items i ON i.id = m.item_id WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count maintenance records: %w", err)
	}
	limit, offset := arg(p.Limit), arg(p.Offset)
	rows, err := r.pool.Query(ctx, selectRecord+` WHERE `+where+`
		ORDER BY (m.status = 'open') DESC, m.reported_at DESC, m.id DESC
		LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list maintenance records: %w", err)
	}
	out, err := collect(rows)
	return out, total, err
}

// dueExpr is when an asset's maintenance falls due: the last maintenance (or
// creation) plus its interval in 24-hour days, matching domain.AddDays.
const dueExpr = `(COALESCE(i.last_maintenance_at, i.created_at) + i.maintenance_interval_days * interval '24 hours')`

// Due implements app.Repository.
func (r *Repo) Due(ctx context.Context, f domain.DueFilter, p domain.Page) ([]domain.DueItem, int, error) {
	conds := []string{"i.deleted_at IS NULL", "i.kind = 'asset'", "i.maintenance_interval_days IS NOT NULL"}
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	conds = append(conds, dueExpr+" <= "+arg(domain.AddDays(f.Now, f.WithinDays)))
	if f.CategoryID != nil {
		conds = append(conds, "i.category_id = "+arg(*f.CategoryID))
	}
	if f.Q != "" {
		q := arg(escapeLike(f.Q))
		conds = append(conds, fmt.Sprintf(
			`(i.code ILIKE '%%' || %s || '%%' ESCAPE '\' OR i.description ILIKE '%%' || %s || '%%' ESCAPE '\')`, q, q))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM items i WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count due items: %w", err)
	}
	limit, offset := arg(p.Limit), arg(p.Offset)
	rows, err := r.pool.Query(ctx, `
		SELECT i.id::text, i.code, i.description, i.category_id, c.name, i.status, i.location_id, l.name,
		       i.last_maintenance_at, i.maintenance_interval_days, `+dueExpr+`
		FROM items i
		JOIN categories c ON c.id = i.category_id
		LEFT JOIN locations l ON l.id = i.location_id
		WHERE `+where+`
		ORDER BY `+dueExpr+` ASC, i.code ASC
		LIMIT `+limit+` OFFSET `+offset, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list due items: %w", err)
	}
	defer rows.Close()
	out := []domain.DueItem{}
	for rows.Next() {
		var d domain.DueItem
		var status string
		if err := rows.Scan(&d.ItemID, &d.Code, &d.Description, &d.CategoryID, &d.CategoryName, &status, &d.LocationID,
			&d.LocationName, &d.LastMaintenanceAt, &d.IntervalDays, &d.DueAt); err != nil {
			return nil, 0, fmt.Errorf("scan due item: %w", err)
		}
		d.Status = domain.ItemStatus(status)
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
