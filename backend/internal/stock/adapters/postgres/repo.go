// Package postgres implements the stock repository on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	qtyval "github.com/edgar/inventario/backend/internal/platform/quantity"

	"github.com/edgar/inventario/backend/internal/platform/db"
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
// the read until commit, so concurrent movements on one item serialize (two
// opposite transfers cannot deadlock: they queue on the same row) and the
// stock check in decide can never be raced. For consumables the per-location
// rows, the item total and the movement are written in the same transaction;
// a row that reaches quantity 0 is deleted.
func (r *Repo) Record(ctx context.Context, itemID, userID string, decide func(domain.ItemState) (domain.Outcome, error)) (domain.Movement, []domain.LocationStock, error) {
	return r.record(ctx, itemID, userID, decide, nil, nil)
}

// RecordExecutionMaterial records a delivery from or return to inventory for
// one execution while consuming only that execution's own reservation.
func (r *Repo) RecordExecutionMaterial(ctx context.Context, itemID, userID string, executionID int64, requestID, fingerprint string, decide func(domain.ItemState) (domain.Outcome, error)) (domain.Movement, []domain.LocationStock, error) {
	return r.record(ctx, itemID, userID, decide, &executionID, &materialIdempotency{RequestID: requestID, Fingerprint: fingerprint})
}

type materialIdempotency struct {
	RequestID   string
	Fingerprint string
}

func (r *Repo) record(ctx context.Context, itemID, userID string, decide func(domain.ItemState) (domain.Outcome, error), executionID *int64, idempotency *materialIdempotency) (domain.Movement, []domain.LocationStock, error) {
	if !validUUID(itemID) {
		return domain.Movement{}, nil, domain.ErrItemNotFound
	}
	if idempotency != nil && !validUUID(idempotency.RequestID) {
		return domain.Movement{}, nil, domain.FieldError("request_id", "must be a UUID")
	}
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Movement{}, nil, fmt.Errorf("begin movement: %w", err)
	}
	defer rollback()
	if idempotency != nil {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, idempotency.RequestID); err != nil {
			return domain.Movement{}, nil, err
		}
	}

	var cur domain.ItemState
	var kind string
	err = tx.QueryRow(ctx,
		`SELECT kind, stock_current, location_id FROM items WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`,
		itemID).Scan(&kind, &cur.StockCurrent, &cur.LocationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Movement{}, nil, domain.ErrItemNotFound
	}
	if err != nil {
		return domain.Movement{}, nil, fmt.Errorf("lock item: %w", err)
	}
	cur.Kind = domain.Kind(kind)
	if idempotency != nil {
		var fingerprint string
		var movementID int64
		err := tx.QueryRow(ctx, `SELECT fingerprint,movement_id FROM manufacturing_execution_material_operations WHERE request_id=$1::uuid`, idempotency.RequestID).Scan(&fingerprint, &movementID)
		if err == nil {
			if fingerprint != idempotency.Fingerprint {
				return domain.Movement{}, nil, domain.ErrIdempotencyConflict
			}
			var movement domain.Movement
			var movementType string
			if err := tx.QueryRow(ctx, `SELECT m.id,m.item_id::text,m.type,m.qty,m.stock_before,m.stock_after,m.location_id,
m.from_location_id,m.to_location_id,m.moved_qty,m.user_id::text,u.name,m.note,m.created_at
FROM stock_movements m LEFT JOIN users u ON u.id=m.user_id WHERE m.id=$1`, movementID).
				Scan(&movement.ID, &movement.ItemID, &movementType, &movement.Qty, &movement.StockBefore, &movement.StockAfter, &movement.LocationID,
					&movement.FromLocationID, &movement.ToLocationID, &movement.MovedQty, &movement.UserID, &movement.UserName, &movement.Note, &movement.CreatedAt); err != nil {
				return domain.Movement{}, nil, err
			}
			movement.Type = domain.MovementType(movementType)
			var stocks []domain.LocationStock
			if cur.Kind == domain.KindConsumable {
				stocks, err = loadStocks(ctx, tx, itemID)
				if err != nil {
					return domain.Movement{}, nil, err
				}
			}
			if err := commit(); err != nil {
				return domain.Movement{}, nil, err
			}
			return movement, stocks, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return domain.Movement{}, nil, err
		}
	}
	// The item row lock serializes this with custody assign/return, which
	// lock the same row, so the flag cannot go stale before commit.
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM custody_assignments WHERE item_id = $1::uuid AND returned_at IS NULL)`,
		itemID).Scan(&cur.OnLoan); err != nil {
		return domain.Movement{}, nil, fmt.Errorf("check active loan: %w", err)
	}
	if cur.Locations, err = loadLocations(ctx, tx); err != nil {
		return domain.Movement{}, nil, err
	}
	if cur.Kind == domain.KindConsumable {
		if cur.Stocks, err = loadStocks(ctx, tx, itemID); err != nil {
			return domain.Movement{}, nil, err
		}
		if len(cur.Stocks) == 0 {
			err = tx.QueryRow(ctx, `
				SELECT COALESCE(location_id, to_location_id) FROM stock_movements
				WHERE item_id = $1::uuid AND COALESCE(location_id, to_location_id) IS NOT NULL
				ORDER BY id DESC LIMIT 1`, itemID).Scan(&cur.LastLocationID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return domain.Movement{}, nil, fmt.Errorf("find last location: %w", err)
			}
		}
	}

	out, err := decide(cur)
	if err != nil {
		return domain.Movement{}, nil, err
	}
	m := out.Movement
	var materialOperation string
	if executionID != nil {
		if m.Type != domain.TypeOut && m.Type != domain.TypeIn {
			return domain.Movement{}, nil, domain.ErrInvalidInput
		}
		var status string
		var required, reserved, delivered, returned qtyval.Quantity
		err := tx.QueryRow(ctx, `SELECT e.status,m.quantity_required,m.quantity_reserved,m.quantity_delivered,m.quantity_returned
FROM manufacturing_execution_materials m JOIN manufacturing_executions e ON e.id=m.execution_id
WHERE m.execution_id=$1 AND m.item_id=$2::uuid FOR UPDATE OF m,e`, *executionID, itemID).
			Scan(&status, &required, &reserved, &delivered, &returned)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Movement{}, nil, domain.ErrReservationUnavailable
		}
		if err != nil {
			return domain.Movement{}, nil, err
		}
		if status != "registered" && status != "in_progress" && status != "paused" {
			return domain.Movement{}, nil, domain.ErrExecutionInactive
		}
		qty := out.Movement.Qty
		if out.Movement.Type == domain.TypeOut {
			qty = -qty
			if qty > reserved || delivered+qty > required {
				return domain.Movement{}, nil, domain.ErrReservationUnavailable
			}
			materialOperation = "delivery"
		} else {
			if qty <= 0 || qty > delivered-returned {
				return domain.Movement{}, nil, domain.ErrReservationUnavailable
			}
			materialOperation = "return"
		}
		if materialOperation == "delivery" {
			if _, err := tx.Exec(ctx, `UPDATE manufacturing_execution_materials
AS m SET quantity_reserved=m.quantity_reserved-$3,quantity_delivered=m.quantity_delivered+$3,
quantity_earmarked_consumed=m.quantity_earmarked_consumed+
LEAST($3,GREATEST(0,COALESCE((
    SELECT SUM(LEAST(link.quantity,l.received_qty))
    FROM manufacturing_execution_purchase_links link
    JOIN manufacturing_execution_shortage_requests r ON r.id=link.shortage_request_id
    JOIN purchase_order_lines l ON l.id=link.purchase_order_line_id
    WHERE r.execution_id=m.execution_id AND r.status='open' AND l.item_id=m.item_id
),0)-m.quantity_earmarked_consumed))
WHERE m.execution_id=$1 AND m.item_id=$2::uuid`, *executionID, itemID, qty); err != nil {
				return domain.Movement{}, nil, err
			}
		} else if _, err := tx.Exec(ctx, `UPDATE manufacturing_execution_materials
SET quantity_returned=quantity_returned+$3
WHERE execution_id=$1 AND item_id=$2::uuid`, *executionID, itemID, qty); err != nil {
			return domain.Movement{}, nil, err
		}
		prefix := fmt.Sprintf("Execution %d material %s", *executionID, materialOperation)
		if m.Note == "" {
			m.Note = prefix
		} else {
			m.Note = prefix + ": " + m.Note
			if len(m.Note) > domain.MaxNoteLen {
				return domain.Movement{}, nil, domain.FieldError("note", "too long after execution reference")
			}
		}
	} else if m.Type == domain.TypeOut && m.Qty < 0 {
		var reserved qtyval.Quantity
		if err := tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity_reserved),0) FROM manufacturing_execution_materials WHERE item_id=$1::uuid`, itemID).Scan(&reserved); err != nil {
			return domain.Movement{}, nil, err
		}
		available := cur.StockCurrent - reserved
		requested := -m.Qty
		if requested > available {
			return domain.Movement{}, nil, &domain.InsufficientStockError{Available: available, Requested: requested}
		}
	}
	if cur.Kind == domain.KindConsumable {
		if out.DefaultLocation {
			id, err := db.EnsureDefaultLocation(ctx, tx)
			if err != nil {
				return domain.Movement{}, nil, err
			}
			m.LocationID = &id
			for i := range out.Changes {
				if out.Changes[i].LocationID == 0 {
					out.Changes[i].LocationID = id
				}
			}
		}
		if err := writeStocks(ctx, tx, itemID, out.Changes); err != nil {
			return domain.Movement{}, nil, mapFK(err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE items SET stock_current = $2, updated_at = now() WHERE id = $1::uuid`,
			itemID, out.StockCurrent); err != nil {
			return domain.Movement{}, nil, fmt.Errorf("update item: %w", err)
		}
	} else if _, err := tx.Exec(ctx,
		`UPDATE items SET stock_current = $2, location_id = $3, updated_at = now() WHERE id = $1::uuid`,
		itemID, out.StockCurrent, out.LocationID); err != nil {
		return domain.Movement{}, nil, mapFK(fmt.Errorf("update item: %w", err))
	}
	if cur.Kind == domain.KindConsumable && (m.Type == domain.TypeIn || m.Type == domain.TypeAdjust || materialOperation != "") {
		if _, err := tx.Exec(ctx, `SELECT rebalance_manufacturing_execution_materials($1::uuid)`, itemID); err != nil {
			return domain.Movement{}, nil, fmt.Errorf("rebalance execution reservations: %w", err)
		}
	}
	m.ItemID = itemID
	m.UserID = &userID
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, location_id, from_location_id,
			to_location_id, moved_qty, user_id, note)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10::uuid, $11)
		RETURNING id, created_at, (SELECT name FROM users WHERE id = $10::uuid)`,
		itemID, string(m.Type), m.Qty, m.StockBefore, m.StockAfter, m.LocationID, m.FromLocationID, m.ToLocationID,
		m.MovedQty, userID, m.Note,
	).Scan(&m.ID, &m.CreatedAt, &m.UserName)
	if err != nil {
		return domain.Movement{}, nil, mapFK(fmt.Errorf("insert movement: %w", err))
	}
	if idempotency != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_execution_material_operations(request_id,execution_id,item_id,movement_id,fingerprint)
VALUES($1::uuid,$2,$3::uuid,$4,$5)`, idempotency.RequestID, *executionID, itemID, m.ID, idempotency.Fingerprint); err != nil {
			return domain.Movement{}, nil, err
		}
	}
	var stocks []domain.LocationStock
	if cur.Kind == domain.KindConsumable {
		if stocks, err = loadStocks(ctx, tx, itemID); err != nil {
			return domain.Movement{}, nil, err
		}
	}
	if err := commit(); err != nil {
		return domain.Movement{}, nil, fmt.Errorf("commit movement: %w", err)
	}
	return m, stocks, nil
}

// loadLocations returns every location id with its name.
func loadLocations(ctx context.Context, tx pgx.Tx) (map[int64]string, error) {
	rows, err := tx.Query(ctx, `SELECT id, name FROM locations`)
	if err != nil {
		return nil, fmt.Errorf("load locations: %w", err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

// loadStocks returns an item's per-location stock ordered by location name.
func loadStocks(ctx context.Context, tx pgx.Tx, itemID string) ([]domain.LocationStock, error) {
	return itemStocks(ctx, tx, itemID)
}

// writeStocks upserts the changed quantities and deletes rows that reached 0.
func writeStocks(ctx context.Context, tx pgx.Tx, itemID string, changes []domain.StockChange) error {
	for _, c := range changes {
		var err error
		if c.Quantity == 0 {
			_, err = tx.Exec(ctx, `DELETE FROM item_stocks WHERE item_id = $1::uuid AND location_id = $2`, itemID, c.LocationID)
		} else {
			_, err = tx.Exec(ctx, `
				INSERT INTO item_stocks (item_id, location_id, quantity) VALUES ($1::uuid, $2, $3)
				ON CONFLICT (item_id, location_id) DO UPDATE SET quantity = EXCLUDED.quantity, updated_at = now()`,
				itemID, c.LocationID, c.Quantity)
		}
		if err != nil {
			return fmt.Errorf("write stock: %w", err)
		}
	}
	return nil
}

// mapFK turns a location foreign-key violation into ErrLocationNotFound.
func mapFK(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKey {
		switch pgErr.ConstraintName {
		case "items_location_id_fkey", "item_stocks_location_id_fkey", "stock_movements_location_id_fkey",
			"stock_movements_from_location_id_fkey", "stock_movements_to_location_id_fkey":
			return domain.ErrLocationNotFound
		}
	}
	return err
}

// RecordOpenings appends one opening movement per location, each carrying
// the running total, so the history reconciles with items.stock_current.
func RecordOpenings(ctx context.Context, tx pgx.Tx, itemID string, stocks []domain.LocationStock, userID string) error {
	var uid *string
	if userID != "" {
		uid = &userID
	}
	var before qtyval.Quantity
	for _, st := range stocks {
		if st.Quantity <= 0 {
			continue
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, location_id, user_id, note)
			VALUES ($1::uuid, 'opening', $2, $3, $4, $5, $6::uuid, 'Opening balance')`,
			itemID, st.Quantity, before, before+st.Quantity, st.LocationID, uid)
		if err != nil {
			return fmt.Errorf("record opening movement: %w", err)
		}
		before += st.Quantity
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
		SELECT m.id, m.item_id::text, m.type, m.qty, m.stock_before, m.stock_after, m.location_id,
		       m.from_location_id, m.to_location_id, m.moved_qty, m.user_id::text, u.name, m.note, m.created_at
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
		if err := rows.Scan(&m.ID, &m.ItemID, &t, &m.Qty, &m.StockBefore, &m.StockAfter, &m.LocationID,
			&m.FromLocationID, &m.ToLocationID, &m.MovedQty, &m.UserID, &m.UserName, &m.Note, &m.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan movement: %w", err)
		}
		m.Type = domain.MovementType(t)
		out = append(out, m)
	}
	return out, total, rows.Err()
}

// Alerts implements app.MovementRepository. The alert is about the total;
// each one also carries the per-location breakdown, and the single location
// fields are set only when all the stock sits in one place.
func (r *Repo) Alerts(ctx context.Context, p domain.Page) ([]domain.Alert, int, error) {
	const cond = `i.deleted_at IS NULL AND i.kind = 'consumable' AND i.stock_min > 0 AND i.stock_current <= i.stock_min`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM items i WHERE `+cond).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count alerts: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT i.id::text, i.code, i.description, COALESCE(i.unit, ''), i.stock_current, i.stock_min
		FROM items i
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
		if err := rows.Scan(&a.ItemID, &a.Code, &a.Description, &a.Unit, &a.StockCurrent, &a.StockMin); err != nil {
			return nil, 0, fmt.Errorf("scan alert: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	for i := range out {
		stocks, err := itemStocks(ctx, r.pool, out[i].ItemID)
		if err != nil {
			return nil, 0, err
		}
		out[i].Stocks = stocks
		if len(stocks) == 1 {
			id, name := stocks[0].LocationID, stocks[0].LocationName
			out[i].LocationID, out[i].LocationName = &id, &name
		}
	}
	return out, total, nil
}

// querier is satisfied by pgx.Tx and *pgxpool.Pool.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func itemStocks(ctx context.Context, q querier, itemID string) ([]domain.LocationStock, error) {
	rows, err := q.Query(ctx, `
		SELECT s.location_id, l.name, s.quantity
		FROM item_stocks s JOIN locations l ON l.id = s.location_id
		WHERE s.item_id = $1::uuid
		ORDER BY lower(l.name), l.id`, itemID)
	if err != nil {
		return nil, fmt.Errorf("load stocks: %w", err)
	}
	defer rows.Close()
	out := []domain.LocationStock{}
	for rows.Next() {
		var st domain.LocationStock
		if err := rows.Scan(&st.LocationID, &st.LocationName, &st.Quantity); err != nil {
			return nil, fmt.Errorf("scan stock: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
