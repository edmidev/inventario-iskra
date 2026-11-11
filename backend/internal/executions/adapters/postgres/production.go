package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5"
)

type unitQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

const selectFinishedUnits = `SELECT u.id,u.code,u.execution_id,u.produced_at,d.id,d.customer_id,d.delivered_at
FROM finished_product_units u
LEFT JOIN manufacturing_delivery_units du ON du.unit_id=u.id
LEFT JOIN manufacturing_deliveries d ON d.id=du.delivery_id`

func queryUnits(ctx context.Context, q unitQueryer, sql string, args ...any) ([]domain.FinishedUnit, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	units := []domain.FinishedUnit{}
	for rows.Next() {
		var unit domain.FinishedUnit
		if err := rows.Scan(&unit.ID, &unit.Code, &unit.ExecutionID, &unit.ProducedAt, &unit.DeliveryID, &unit.CustomerID, &unit.DeliveredAt); err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, rows.Err()
}

func (r *Repo) ListUnits(ctx context.Context, executionID int64) ([]domain.FinishedUnit, error) {
	var id int64
	if err := r.pool.QueryRow(ctx, `SELECT id FROM manufacturing_executions WHERE id=$1`, executionID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return queryUnits(ctx, r.pool, selectFinishedUnits+` WHERE u.execution_id=$1 ORDER BY u.id`, executionID)
}

// Finished stock is derived from serialized production and delivery records;
// it never shares consumable or tool balances.
func (r *Repo) ListFinishedStock(ctx context.Context, filter domain.FinishedStockFilter) ([]domain.FinishedStockRow, int, error) {
	pattern := ""
	if filter.Q != "" {
		pattern = "%" + escape.Replace(filter.Q) + "%"
	}
	from := ` FROM finished_product_units u
JOIN manufacturing_executions e ON e.id=u.execution_id
LEFT JOIN manufacturing_delivery_units du ON du.unit_id=u.id
LEFT JOIN manufacturing_deliveries d ON d.id=du.delivery_id
WHERE ($1='' OR ($1='available' AND d.id IS NULL) OR ($1='delivered' AND d.id IS NOT NULL))
AND ($2='' OR u.code ILIKE $2 ESCAPE '\' OR e.project_snapshot->>'project_name' ILIKE $2 ESCAPE '\')`
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*)`+from, filter.Status, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `SELECT u.id,u.code,u.execution_id,e.project_snapshot->>'project_name',u.produced_at,
CASE WHEN d.id IS NULL THEN 'available' ELSE 'delivered' END,COALESCE(d.number,''),COALESCE(d.customer_name,''),d.delivered_at`+from+` ORDER BY u.id DESC LIMIT $3 OFFSET $4`, filter.Status, pattern, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := []domain.FinishedStockRow{}
	for rows.Next() {
		var row domain.FinishedStockRow
		if err := rows.Scan(&row.ID, &row.Code, &row.ExecutionID, &row.ProjectName, &row.ProducedAt, &row.Status, &row.Receipt, &row.CustomerName, &row.DeliveredAt); err != nil {
			return nil, 0, err
		}
		result = append(result, row)
	}
	return result, total, rows.Err()
}

// Produce serializes production for one execution so concurrent requests cannot
// exceed its planned count. A request ID can be replayed without creating units.
func (r *Repo) Produce(ctx context.Context, executionID int64, actorID string, input domain.ProduceInput) ([]domain.FinishedUnit, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return nil, err
	}
	defer rollback()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, input.RequestID); err != nil {
		return nil, err
	}
	var priorExecutionID int64
	var priorQuantity int
	var priorActorID string
	err = tx.QueryRow(ctx, `SELECT execution_id,quantity,actor_id::text FROM manufacturing_production_batches WHERE request_id=$1::uuid`, input.RequestID).Scan(&priorExecutionID, &priorQuantity, &priorActorID)
	if err == nil {
		if priorExecutionID != executionID || priorQuantity != input.Quantity || priorActorID != actorID {
			return nil, domain.ErrProductionReplay
		}
		units, err := queryUnits(ctx, tx, selectFinishedUnits+` WHERE u.production_request_id=$1::uuid ORDER BY u.id`, input.RequestID)
		if err != nil {
			return nil, err
		}
		return units, commit()
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var status string
	var planned int
	err = tx.QueryRow(ctx, `SELECT status,units FROM manufacturing_executions WHERE id=$1 FOR UPDATE`, executionID).Scan(&status, &planned)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != string(domain.StatusInProgress) {
		return nil, domain.ErrInvalidState
	}
	var produced int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM finished_product_units WHERE execution_id=$1`, executionID).Scan(&produced); err != nil {
		return nil, err
	}
	if input.Quantity > planned-produced {
		return nil, domain.ErrProductionLimit
	}
	if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_production_batches(request_id,execution_id,quantity,actor_id) VALUES($1::uuid,$2,$3,$4::uuid)`, input.RequestID, executionID, input.Quantity, actorID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO finished_product_units(execution_id,production_request_id)
SELECT $1,$2::uuid FROM generate_series(1,$3)`, executionID, input.RequestID, input.Quantity); err != nil {
		return nil, err
	}
	if err := insertAutomaticEvent(ctx, tx, executionID, actorID, "production", fmt.Sprintf("Se registraron %d unidades terminadas.", input.Quantity)); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE manufacturing_executions SET updated_at=now() WHERE id=$1`, executionID); err != nil {
		return nil, err
	}
	units, err := queryUnits(ctx, tx, selectFinishedUnits+` WHERE u.production_request_id=$1::uuid ORDER BY u.id`, input.RequestID)
	if err != nil {
		return nil, err
	}
	return units, commit()
}
