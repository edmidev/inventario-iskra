// Package postgres implements the dashboard read model on PostgreSQL via pgx.
//
// It is a read-only projection over tables owned by catalog, custody and
// maintenance; the SQL mirrors their rules (see the comments per query) so the
// dashboard never disagrees with the list endpoints. No domain or app package
// of those modules is imported.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/edgar/inventario/backend/internal/dashboard/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repo implements app.Repository.
type Repo struct{ pool *pgxpool.Pool }

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const (
	// activeLoanJoin attaches the item's active loan (at most one, enforced by
	// a partial unique index), as the catalog adapter does.
	activeLoanJoin = `LEFT JOIN custody_assignments ca ON ca.item_id = i.id AND ca.returned_at IS NULL`
	// maintenanceDue mirrors the maintenance module: last maintenance (or
	// creation) plus the interval in 24-hour days.
	maintenanceDue = `(COALESCE(i.last_maintenance_at, i.created_at) + i.maintenance_interval_days * interval '24 hours')`
	// lowStock mirrors the catalog low_stock filter.
	lowStock = `i.kind = 'consumable' AND i.stock_min > 0 AND i.stock_current <= i.stock_min`
	// pendingOrdered is what is still to arrive for the item on open purchase
	// orders (owned by purchasing). A new-item line counts once it has an item.
	pendingOrdered = `COALESCE((SELECT sum(l.quantity - l.received_qty)::int
		FROM purchase_order_lines l JOIN purchase_orders o ON o.id = l.order_id
		WHERE l.item_id = i.id AND o.status IN ('ordered', 'partially_received')), 0)`
)

// Summary implements app.Repository: one counters query, one open-failures
// query and three bounded top lists, all over non-deleted items.
func (r *Repo) Summary(ctx context.Context, now time.Time) (domain.Summary, error) {
	var s domain.Summary
	err := r.pool.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE i.kind = 'asset'),
		       count(*) FILTER (WHERE i.kind = 'consumable'),
		       count(*) FILTER (WHERE i.kind = 'asset' AND i.status = 'functional' AND ca.id IS NULL),
		       count(*) FILTER (WHERE i.kind = 'asset' AND (i.status <> 'functional' OR ca.id IS NOT NULL)),
		       count(*) FILTER (WHERE i.kind = 'asset' AND ca.id IS NOT NULL),
		       count(*) FILTER (WHERE i.kind = 'asset' AND i.status = 'inoperative'),
		       count(*) FILTER (WHERE i.kind = 'asset' AND i.status = 'in_repair'),
		       count(*) FILTER (WHERE `+lowStock+`),
		       count(*) FILTER (WHERE ca.id IS NOT NULL),
		       count(*) FILTER (WHERE ca.due_at < $1),
		       count(*) FILTER (WHERE i.kind = 'asset' AND i.maintenance_interval_days IS NOT NULL AND `+maintenanceDue+` < $1)
		FROM items i `+activeLoanJoin+`
		WHERE i.deleted_at IS NULL`, now).Scan(
		&s.Items.Total, &s.Items.Assets, &s.Items.Consumables,
		&s.Assets.Available, &s.Assets.Unavailable, &s.Assets.OnLoan, &s.Assets.Inoperative, &s.Assets.InRepair,
		&s.LowStock.Count, &s.Loans.Active, &s.Loans.Overdue, &s.Maintenance.Overdue)
	if err != nil {
		return domain.Summary{}, fmt.Errorf("dashboard counters: %w", err)
	}
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*) FROM maintenance_records m JOIN items i ON i.id = m.item_id
		WHERE i.deleted_at IS NULL AND m.kind = 'corrective' AND m.status = 'open'`).Scan(&s.Maintenance.OpenFailures); err != nil {
		return domain.Summary{}, fmt.Errorf("dashboard open failures: %w", err)
	}
	if s.LowStock.Top, err = r.lowStockTop(ctx); err != nil {
		return domain.Summary{}, err
	}
	if s.Loans.OverdueTop, err = r.overdueLoansTop(ctx, now); err != nil {
		return domain.Summary{}, err
	}
	if s.Maintenance.OverdueTop, err = r.overdueMaintenanceTop(ctx, now); err != nil {
		return domain.Summary{}, err
	}
	return s, nil
}

// collect runs a query and scans every row with scan.
func collect[T any](ctx context.Context, pool *pgxpool.Pool, op, sql string, scan func(pgx.Rows) (T, error), args ...any) ([]T, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: scan: %w", op, err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// lowStockTop lists the most severe first: lowest stock/min ratio, then code.
// Consumables keep no single location: location_name is the location of the
// only stock row, or null when the stock is spread over several (or none),
// and stocks carries the per-location breakdown (table owned by stock).
func (r *Repo) lowStockTop(ctx context.Context) ([]domain.LowStockItem, error) {
	items, err := collect(ctx, r.pool, "dashboard low stock", `
		SELECT i.id::text, i.code, i.description, COALESCE(i.unit, ''), i.stock_current, i.stock_min, `+pendingOrdered+`
		FROM items i
		WHERE i.deleted_at IS NULL AND `+lowStock+`
		ORDER BY i.stock_current::float8 / i.stock_min ASC, i.code ASC
		LIMIT $1`,
		func(row pgx.Rows) (domain.LowStockItem, error) {
			var it domain.LowStockItem
			err := row.Scan(&it.ID, &it.Code, &it.Description, &it.Unit, &it.StockCurrent, &it.StockMin, &it.PendingOrdered)
			return it, err
		}, domain.TopLimit)
	if err != nil || len(items) == 0 {
		return items, err
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	type row struct {
		item string
		loc  domain.StockLocation
	}
	rows, err := collect(ctx, r.pool, "dashboard low stock breakdown", `
		SELECT s.item_id::text, s.location_id, l.name, s.quantity
		FROM item_stocks s JOIN locations l ON l.id = s.location_id
		WHERE s.item_id = ANY($1::uuid[])
		ORDER BY lower(l.name), l.id`,
		func(r pgx.Rows) (row, error) {
			var x row
			err := r.Scan(&x.item, &x.loc.LocationID, &x.loc.LocationName, &x.loc.Quantity)
			return x, err
		}, ids)
	if err != nil {
		return nil, err
	}
	byItem := map[string][]domain.StockLocation{}
	for _, x := range rows {
		byItem[x.item] = append(byItem[x.item], x.loc)
	}
	for i := range items {
		items[i].Stocks = byItem[items[i].ID]
		if len(items[i].Stocks) == 1 {
			name := items[i].Stocks[0].LocationName
			items[i].LocationName = &name
		}
	}
	return items, nil
}

// overdueLoansTop lists the oldest overdue loans first.
func (r *Repo) overdueLoansTop(ctx context.Context, now time.Time) ([]domain.OverdueLoan, error) {
	return collect(ctx, r.pool, "dashboard overdue loans", `
		SELECT i.id::text, i.code, i.description, al.name, ca.due_at
		FROM items i `+activeLoanJoin+`
		JOIN locations al ON al.id = ca.assignee_location_id
		WHERE i.deleted_at IS NULL AND ca.due_at < $1
		ORDER BY ca.due_at ASC, i.code ASC
		LIMIT $2`,
		func(row pgx.Rows) (domain.OverdueLoan, error) {
			var o domain.OverdueLoan
			err := row.Scan(&o.ItemID, &o.Code, &o.Description, &o.AssigneeName, &o.DueAt)
			return o, err
		}, now, domain.TopLimit)
}

// overdueMaintenanceTop lists the most overdue assets first.
func (r *Repo) overdueMaintenanceTop(ctx context.Context, now time.Time) ([]domain.OverdueMaintenance, error) {
	return collect(ctx, r.pool, "dashboard overdue maintenance", `
		SELECT i.id::text, i.code, i.description, `+maintenanceDue+`
		FROM items i
		WHERE i.deleted_at IS NULL AND i.kind = 'asset' AND i.maintenance_interval_days IS NOT NULL
		  AND `+maintenanceDue+` < $1
		ORDER BY `+maintenanceDue+` ASC, i.code ASC
		LIMIT $2`,
		func(row pgx.Rows) (domain.OverdueMaintenance, error) {
			var o domain.OverdueMaintenance
			err := row.Scan(&o.ItemID, &o.Code, &o.Description, &o.MaintenanceDueAt)
			return o, err
		}, now, domain.TopLimit)
}

// ReorderCandidates lists the low-stock consumables, most severe first, with
// the supplier and terms of the item's latest non-draft, non-cancelled order
// line. A deleted or inactive supplier counts as no habitual supplier.
func (r *Repo) ReorderCandidates(ctx context.Context) ([]domain.ReorderCandidate, error) {
	return collect(ctx, r.pool, "dashboard reorder candidates", `
		SELECT i.id::text, i.code, i.description, COALESCE(i.unit, ''), i.stock_current, i.stock_min,
		       `+pendingOrdered+`, h.supplier_id, h.supplier_name, h.unit_cost_minor, h.currency
		FROM items i
		LEFT JOIN LATERAL (
			SELECT o.supplier_id, s.name AS supplier_name, l.unit_cost_minor, o.currency
			FROM purchase_order_lines l
			JOIN purchase_orders o ON o.id = l.order_id
			JOIN suppliers s ON s.id = o.supplier_id AND s.deleted_at IS NULL AND s.active
			WHERE l.item_id = i.id AND o.status IN ('ordered', 'partially_received', 'received')
			ORDER BY o.ordered_at DESC NULLS LAST, o.id DESC
			LIMIT 1) h ON true
		WHERE i.deleted_at IS NULL AND `+lowStock+`
		ORDER BY i.stock_current::float8 / i.stock_min ASC, i.code ASC`,
		func(row pgx.Rows) (domain.ReorderCandidate, error) {
			var c domain.ReorderCandidate
			err := row.Scan(&c.ItemID, &c.Code, &c.Description, &c.Unit, &c.StockCurrent, &c.StockMin,
				&c.PendingOrdered, &c.SupplierID, &c.SupplierName, &c.LastUnitCostMinor, &c.LastCurrency)
			return c, err
		})
}
