// Package postgres implements the purchase order repository on PostgreSQL via pgx.
//
// Reads join the suppliers table (name) and the items table (code of the items
// a receipt created), both read-only and by id.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/purchasing/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pgForeignKey = "23503"

// Repo implements the purchase order repository port.
type Repo struct{ pool *pgxpool.Pool }

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const orderCols = `o.id, o.supplier_id, s.name, o.status, o.currency, o.exchange_rate, o.expected_at,
	o.notes, COALESCE(o.created_by::text, ''), o.created_at, o.updated_at, o.ordered_at, o.cancelled_at`

const orderFrom = `purchase_orders o JOIN suppliers s ON s.id = o.supplier_id`

func scanOrder(row pgx.Row, o *domain.Order) error {
	var status, currency string
	err := row.Scan(&o.ID, &o.SupplierID, &o.SupplierName, &status, &currency, &o.ExchangeRate, &o.ExpectedAt,
		&o.Notes, &o.CreatedBy, &o.CreatedAt, &o.UpdatedAt, &o.OrderedAt, &o.CancelledAt)
	o.Status, o.Currency = domain.Status(status), domain.Currency(currency)
	return err
}

func mapFK(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKey {
		switch pgErr.ConstraintName {
		case "purchase_orders_supplier_id_fkey":
			return domain.FieldError("supplier_id", "supplier does not exist")
		case "po_receipts_location_id_fkey":
			return domain.FieldError("location_id", "location does not exist")
		}
	}
	return err
}

// Create inserts a draft and its lines, filling ids and timestamps.
func (r *Repo) Create(ctx context.Context, o *domain.Order) error {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return fmt.Errorf("begin create order: %w", err)
	}
	defer rollback()
	err = tx.QueryRow(ctx,
		`INSERT INTO purchase_orders (supplier_id, status, currency, exchange_rate, expected_at, notes, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid)
		 RETURNING id, created_at, updated_at`,
		o.SupplierID, string(o.Status), string(o.Currency), o.ExchangeRate, o.ExpectedAt, o.Notes, o.CreatedBy,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return mapFK(err)
	}
	if err := insertLines(ctx, tx, o); err != nil {
		return err
	}
	o.LinesDirty = false
	if err := commit(); err != nil {
		return fmt.Errorf("commit create order: %w", err)
	}
	return nil
}

func insertLines(ctx context.Context, tx pgx.Tx, o *domain.Order) error {
	for i := range o.Lines {
		l := &o.Lines[i]
		var newItem []byte
		if l.NewItem != nil {
			var err error
			if newItem, err = json.Marshal(l.NewItem); err != nil {
				return fmt.Errorf("encode new_item: %w", err)
			}
		}
		l.Position = i + 1
		err := tx.QueryRow(ctx,
			`INSERT INTO purchase_order_lines (order_id, position, item_id, new_item, description, quantity, received_qty, unit_cost_minor)
			 VALUES ($1, $2, NULLIF($3, '')::uuid, $4::jsonb, $5, $6, $7, $8) RETURNING id`,
			o.ID, l.Position, l.ItemID, newItem, l.Description, l.Quantity, l.ReceivedQty, l.UnitCostMinor).Scan(&l.ID)
		if err != nil {
			return fmt.Errorf("insert line %d: %w", i+1, err)
		}
	}
	return nil
}

// Get returns an order with its lines and receipts.
func (r *Repo) Get(ctx context.Context, id int64) (domain.Order, error) {
	return load(ctx, r.pool, id, "")
}

// load reads one order; lock is "" or " FOR UPDATE OF o".
func load(ctx context.Context, q queryer, id int64, lock string) (domain.Order, error) {
	var o domain.Order
	err := scanOrder(q.QueryRow(ctx, `SELECT `+orderCols+` FROM `+orderFrom+` WHERE o.id = $1`+lock, id), &o)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("load order: %w", err)
	}
	lines, err := loadLines(ctx, q, []int64{id})
	if err != nil {
		return domain.Order{}, err
	}
	o.Lines = lines[id]
	if o.Receipts, err = loadReceipts(ctx, q, id); err != nil {
		return domain.Order{}, err
	}
	return o, nil
}

func loadLines(ctx context.Context, q queryer, orderIDs []int64) (map[int64][]domain.Line, error) {
	rows, err := q.Query(ctx,
		`SELECT order_id, id, position, COALESCE(item_id::text, ''), new_item, description, quantity, received_qty, unit_cost_minor
		   FROM purchase_order_lines WHERE order_id = ANY($1) ORDER BY order_id, position, id`, orderIDs)
	if err != nil {
		return nil, fmt.Errorf("load lines: %w", err)
	}
	defer rows.Close()
	out := map[int64][]domain.Line{}
	for rows.Next() {
		var oid int64
		var l domain.Line
		var raw []byte
		if err := rows.Scan(&oid, &l.ID, &l.Position, &l.ItemID, &raw, &l.Description, &l.Quantity, &l.ReceivedQty, &l.UnitCostMinor); err != nil {
			return nil, err
		}
		if raw != nil {
			l.NewItem = &domain.NewItem{}
			if err := json.Unmarshal(raw, l.NewItem); err != nil {
				return nil, fmt.Errorf("decode new_item of line %d: %w", l.ID, err)
			}
		}
		out[oid] = append(out[oid], l)
	}
	return out, rows.Err()
}

func loadReceipts(ctx context.Context, q queryer, orderID int64) ([]domain.Receipt, error) {
	rows, err := q.Query(ctx,
		`SELECT id, location_id, COALESCE(received_by::text, ''), received_at, note, COALESCE(client_key, '')
		   FROM po_receipts WHERE order_id = $1 ORDER BY id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("load receipts: %w", err)
	}
	var out []domain.Receipt
	byID := map[int64]int{}
	for rows.Next() {
		var rc domain.Receipt
		if err := rows.Scan(&rc.ID, &rc.LocationID, &rc.ReceivedBy, &rc.ReceivedAt, &rc.Note, &rc.ClientKey); err != nil {
			rows.Close()
			return nil, err
		}
		byID[rc.ID] = len(out)
		out = append(out, rc)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}

	lrows, err := q.Query(ctx,
		`SELECT rl.receipt_id, rl.id, rl.line_id, rl.quantity
		   FROM po_receipt_lines rl JOIN po_receipts r ON r.id = rl.receipt_id
		  WHERE r.order_id = $1 ORDER BY rl.id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("load receipt lines: %w", err)
	}
	type key struct{ receipt, rline int64 }
	at := map[int64]key{} // receipt line id -> position
	for lrows.Next() {
		var rid, rlid int64
		var rl domain.ReceiptLine
		if err := lrows.Scan(&rid, &rlid, &rl.LineID, &rl.Quantity); err != nil {
			lrows.Close()
			return nil, err
		}
		at[rlid] = key{rid, int64(len(out[byID[rid]].Lines))}
		out[byID[rid]].Lines = append(out[byID[rid]].Lines, rl)
	}
	lrows.Close()
	if err := lrows.Err(); err != nil {
		return nil, err
	}

	irows, err := q.Query(ctx,
		`SELECT ri.receipt_line_id, ri.item_id::text, i.code, ri.quantity
		   FROM po_receipt_items ri
		   JOIN po_receipt_lines rl ON rl.id = ri.receipt_line_id
		   JOIN po_receipts r ON r.id = rl.receipt_id
		   JOIN items i ON i.id = ri.item_id
		  WHERE r.order_id = $1 ORDER BY ri.receipt_line_id, i.code, ri.item_id`, orderID)
	if err != nil {
		return nil, fmt.Errorf("load receipt items: %w", err)
	}
	defer irows.Close()
	for irows.Next() {
		var rlid int64
		var it domain.ReceiptItem
		if err := irows.Scan(&rlid, &it.ItemID, &it.Code, &it.Quantity); err != nil {
			return nil, err
		}
		k := at[rlid]
		rc := &out[byID[k.receipt]]
		rc.Lines[k.rline].Items = append(rc.Lines[k.rline].Items, it)
	}
	return out, irows.Err()
}

// Mutate locks the order row, loads the order, runs fn with a context carrying
// the transaction, and stores the result.
func (r *Repo) Mutate(ctx context.Context, id int64, fn func(context.Context, *domain.Order) error) (domain.Order, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Order{}, fmt.Errorf("begin mutate order: %w", err)
	}
	defer rollback()
	o, err := load(ctx, tx, id, " FOR UPDATE OF o")
	if err != nil {
		return domain.Order{}, err
	}
	if err := fn(db.WithTx(ctx, tx), &o); err != nil {
		if errors.Is(err, domain.ErrNoChange) {
			return o, nil
		}
		return domain.Order{}, err
	}
	if err := save(ctx, tx, &o); err != nil {
		return domain.Order{}, err
	}
	out, err := load(ctx, tx, id, "")
	if err != nil {
		return domain.Order{}, err
	}
	if err := commit(); err != nil {
		return domain.Order{}, fmt.Errorf("commit mutate order: %w", err)
	}
	return out, nil
}

func save(ctx context.Context, tx pgx.Tx, o *domain.Order) error {
	_, err := tx.Exec(ctx,
		`UPDATE purchase_orders
		    SET supplier_id = $2, status = $3, currency = $4, exchange_rate = $5, expected_at = $6,
		        notes = $7, updated_at = $8, ordered_at = $9, cancelled_at = $10
		  WHERE id = $1`,
		o.ID, o.SupplierID, string(o.Status), string(o.Currency), o.ExchangeRate, o.ExpectedAt,
		o.Notes, o.UpdatedAt, o.OrderedAt, o.CancelledAt)
	if err != nil {
		return mapFK(fmt.Errorf("update order: %w", err))
	}
	if o.LinesDirty {
		if _, err := tx.Exec(ctx, `DELETE FROM purchase_order_lines WHERE order_id = $1`, o.ID); err != nil {
			return fmt.Errorf("replace lines: %w", err)
		}
		if err := insertLines(ctx, tx, o); err != nil {
			return err
		}
	} else {
		for _, l := range o.Lines {
			if _, err := tx.Exec(ctx,
				`UPDATE purchase_order_lines SET received_qty = $2, item_id = NULLIF($3, '')::uuid WHERE id = $1`,
				l.ID, l.ReceivedQty, l.ItemID); err != nil {
				return fmt.Errorf("update line %d: %w", l.ID, err)
			}
		}
	}
	for i := range o.Receipts {
		if o.Receipts[i].ID == 0 {
			if err := insertReceipt(ctx, tx, o.ID, &o.Receipts[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertReceipt(ctx context.Context, tx pgx.Tx, orderID int64, rc *domain.Receipt) error {
	err := tx.QueryRow(ctx,
		`INSERT INTO po_receipts (order_id, location_id, received_by, received_at, note, client_key)
		 VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, NULLIF($6, '')) RETURNING id`,
		orderID, rc.LocationID, rc.ReceivedBy, rc.ReceivedAt, rc.Note, rc.ClientKey).Scan(&rc.ID)
	if err != nil {
		return mapFK(fmt.Errorf("insert receipt: %w", err))
	}
	for _, rl := range rc.Lines {
		var rlid int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO po_receipt_lines (receipt_id, line_id, quantity) VALUES ($1, $2, $3) RETURNING id`,
			rc.ID, rl.LineID, rl.Quantity).Scan(&rlid); err != nil {
			return fmt.Errorf("insert receipt line: %w", err)
		}
		for _, it := range rl.Items {
			if _, err := tx.Exec(ctx,
				`INSERT INTO po_receipt_items (receipt_line_id, item_id, quantity) VALUES ($1, $2::uuid, $3)`,
				rlid, it.ItemID, it.Quantity); err != nil {
				return fmt.Errorf("insert receipt item: %w", err)
			}
		}
	}
	return nil
}

// escapeLike makes user text literal inside an ILIKE pattern.
var escapeLike = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// List returns a page of orders (newest first) with their lines, and the total.
func (r *Repo) List(ctx context.Context, f domain.Filter) ([]domain.Order, int, error) {
	where := []string{"TRUE"}
	args := []any{}
	if f.Status != "" {
		args = append(args, string(f.Status))
		where = append(where, fmt.Sprintf("o.status = $%d", len(args)))
	}
	if f.SupplierID != 0 {
		args = append(args, f.SupplierID)
		where = append(where, fmt.Sprintf("o.supplier_id = $%d", len(args)))
	}
	if q := strings.TrimPrefix(f.Q, "#"); q != "" {
		args = append(args, "%"+escapeLike.Replace(q)+"%", q)
		where = append(where, fmt.Sprintf("(s.name ILIKE $%d OR o.id::text = $%d)", len(args)-1, len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM `+orderFrom+` WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT `+orderCols+` FROM `+orderFrom+` WHERE %s ORDER BY o.id DESC LIMIT $%d OFFSET $%d`,
			cond, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	out := []domain.Order{}
	ids := []int64{}
	for rows.Next() {
		var o domain.Order
		if err := scanOrder(rows, &o); err != nil {
			rows.Close()
			return nil, 0, err
		}
		out = append(out, o)
		ids = append(ids, o.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	lines, err := loadLines(ctx, r.pool, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		out[i].Lines = lines[out[i].ID]
	}
	return out, total, nil
}
