// Package postgres implements the stock repository on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo implements app.MovementRepository on one pool.
type Repo struct{ pool *pgxpool.Pool }

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const pgForeignKey = "23503"

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

// Record implements app.MovementRepository. The item row stays locked from
// the read until commit, so concurrent movements on one item serialize and
// the stock check in decide can never be raced.
func (r *Repo) Record(ctx context.Context, itemID, userID string, decide func(domain.ItemState) (domain.Outcome, error)) (domain.Movement, error) {
	if !validUUID(itemID) {
		return domain.Movement{}, domain.ErrItemNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Movement{}, fmt.Errorf("begin movement: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var cur domain.ItemState
	var kind string
	err = tx.QueryRow(ctx,
		`SELECT kind, stock_current, location_id FROM items WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`,
		itemID).Scan(&kind, &cur.StockCurrent, &cur.LocationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Movement{}, domain.ErrItemNotFound
	}
	if err != nil {
		return domain.Movement{}, fmt.Errorf("lock item: %w", err)
	}
	cur.Kind = domain.Kind(kind)

	out, err := decide(cur)
	if err != nil {
		return domain.Movement{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE items SET stock_current = $2, location_id = $3, updated_at = now() WHERE id = $1::uuid`,
		itemID, out.StockCurrent, out.LocationID); err != nil {
		return domain.Movement{}, mapFK(fmt.Errorf("update item: %w", err))
	}
	m := out.Movement
	m.ItemID = itemID
	m.UserID = &userID
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, from_location_id, to_location_id, user_id, note)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::uuid, $9)
		RETURNING id, created_at, (SELECT name FROM users WHERE id = $8::uuid)`,
		itemID, string(m.Type), m.Qty, m.StockBefore, m.StockAfter, m.FromLocationID, m.ToLocationID, userID, m.Note,
	).Scan(&m.ID, &m.CreatedAt, &m.UserName)
	if err != nil {
		return domain.Movement{}, mapFK(fmt.Errorf("insert movement: %w", err))
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Movement{}, fmt.Errorf("commit movement: %w", err)
	}
	return m, nil
}

// mapFK turns a location foreign-key violation into ErrLocationNotFound.
func mapFK(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKey &&
		(pgErr.ConstraintName == "items_location_id_fkey" || pgErr.ConstraintName == "stock_movements_to_location_id_fkey") {
		return domain.ErrLocationNotFound
	}
	return err
}

// RecordOpening appends the opening movement for a freshly created
// consumable inside the caller's transaction (see catalog's create hook).
// userID may be empty for system writes.
func RecordOpening(ctx context.Context, tx pgx.Tx, itemID string, qty int, locationID *int64, userID string) error {
	var uid *string
	if userID != "" {
		uid = &userID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, to_location_id, user_id, note)
		VALUES ($1::uuid, 'opening', $2, 0, $2, $3, $4::uuid, 'Opening balance')`,
		itemID, qty, locationID, uid)
	if err != nil {
		return fmt.Errorf("record opening movement: %w", err)
	}
	return nil
}

// List implements app.MovementRepository.
func (r *Repo) List(ctx context.Context, itemID string, typ domain.MovementType, p domain.Page) ([]domain.Movement, int, error) {
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
		`SELECT count(*) FROM stock_movements WHERE item_id = $1::uuid AND ($2 = '' OR type = $2)`,
		itemID, string(typ)).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count movements: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT m.id, m.item_id::text, m.type, m.qty, m.stock_before, m.stock_after, m.from_location_id,
		       m.to_location_id, m.user_id::text, u.name, m.note, m.created_at
		FROM stock_movements m LEFT JOIN users u ON u.id = m.user_id
		WHERE m.item_id = $1::uuid AND ($2 = '' OR m.type = $2)
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $3 OFFSET $4`, itemID, string(typ), p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list movements: %w", err)
	}
	defer rows.Close()
	out := []domain.Movement{}
	for rows.Next() {
		var m domain.Movement
		var t string
		if err := rows.Scan(&m.ID, &m.ItemID, &t, &m.Qty, &m.StockBefore, &m.StockAfter, &m.FromLocationID,
			&m.ToLocationID, &m.UserID, &m.UserName, &m.Note, &m.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan movement: %w", err)
		}
		m.Type = domain.MovementType(t)
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// Alerts implements app.MovementRepository.
func (r *Repo) Alerts(ctx context.Context, p domain.Page) ([]domain.Alert, int, error) {
	const cond = `i.deleted_at IS NULL AND i.kind = 'consumable' AND i.stock_min > 0 AND i.stock_current <= i.stock_min`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM items i WHERE `+cond).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count alerts: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT i.id::text, i.code, i.description, COALESCE(i.unit, ''), i.stock_current, i.stock_min, i.location_id, l.name
		FROM items i LEFT JOIN locations l ON l.id = i.location_id
		WHERE `+cond+`
		ORDER BY (i.stock_current - i.stock_min) ASC, i.code ASC
		LIMIT $1 OFFSET $2`, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()
	out := []domain.Alert{}
	for rows.Next() {
		var a domain.Alert
		if err := rows.Scan(&a.ItemID, &a.Code, &a.Description, &a.Unit, &a.StockCurrent, &a.StockMin,
			&a.LocationID, &a.LocationName); err != nil {
			return nil, 0, fmt.Errorf("scan alert: %w", err)
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
