package postgres

import (
	"context"
	"errors"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5"
)

const selectDelivery = `SELECT d.id,d.number,d.execution_id,d.customer_id,d.customer_name,d.actor_id::text,d.delivered_at,
ARRAY(SELECT u.code FROM manufacturing_delivery_units du JOIN finished_product_units u ON u.id=du.unit_id
 WHERE du.delivery_id=d.id ORDER BY u.id)::TEXT[]
FROM manufacturing_deliveries d`

func scanDelivery(row pgx.Row) (domain.DeliveryReceipt, error) {
	var receipt domain.DeliveryReceipt
	err := row.Scan(&receipt.ID, &receipt.Number, &receipt.ExecutionID, &receipt.CustomerID, &receipt.CustomerName, &receipt.ActorID, &receipt.DeliveredAt, &receipt.UnitCodes)
	return receipt, err
}

func (r *Repo) ListDeliveries(ctx context.Context, executionID int64) ([]domain.DeliveryReceipt, error) {
	var id int64
	if err := r.pool.QueryRow(ctx, `SELECT id FROM manufacturing_executions WHERE id=$1`, executionID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	rows, err := r.pool.Query(ctx, selectDelivery+` WHERE d.execution_id=$1 ORDER BY d.id`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	receipts := []domain.DeliveryReceipt{}
	for rows.Next() {
		receipt, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	return receipts, rows.Err()
}

// DeliverFinishedUnits moves selected serialized units out of finished stock.
// Distinct requests lock unit rows in numeric order, so a unit can appear on
// exactly one delivery even when clients submit concurrently.
func (r *Repo) DeliverFinishedUnits(ctx context.Context, executionID int64, actorID string, input domain.DeliveryInput, fingerprint string) (domain.DeliveryReceipt, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.DeliveryReceipt{}, err
	}
	defer rollback()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('finished-delivery:' || $1::text,0))`, input.RequestID); err != nil {
		return domain.DeliveryReceipt{}, err
	}
	var priorID int64
	var priorFingerprint string
	err = tx.QueryRow(ctx, `SELECT id,fingerprint FROM manufacturing_deliveries WHERE request_id=$1::uuid`, input.RequestID).Scan(&priorID, &priorFingerprint)
	if err == nil {
		if priorFingerprint != fingerprint {
			return domain.DeliveryReceipt{}, domain.ErrDeliveryReplay
		}
		receipt, err := scanDelivery(tx.QueryRow(ctx, selectDelivery+` WHERE d.id=$1`, priorID))
		if err != nil {
			return domain.DeliveryReceipt{}, err
		}
		return receipt, commit()
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryReceipt{}, err
	}
	var customerName string
	err = tx.QueryRow(ctx, `SELECT c.name FROM manufacturing_execution_customers ec
JOIN customers c ON c.id=ec.customer_id
WHERE ec.execution_id=$1 AND ec.customer_id=$2 FOR SHARE OF ec,c`, executionID, input.CustomerID).Scan(&customerName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeliveryReceipt{}, domain.ErrCustomerNotAssigned
	}
	if err != nil {
		return domain.DeliveryReceipt{}, err
	}
	for _, unitID := range input.UnitIDs {
		var lockedID int64
		err := tx.QueryRow(ctx, `SELECT id FROM finished_product_units WHERE id=$1 AND execution_id=$2 FOR UPDATE`, unitID, executionID).Scan(&lockedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.DeliveryReceipt{}, domain.ErrUnitUnavailable
		}
		if err != nil {
			return domain.DeliveryReceipt{}, err
		}
	}
	var alreadyDelivered bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM manufacturing_delivery_units WHERE unit_id=ANY($1::BIGINT[]))`, input.UnitIDs).Scan(&alreadyDelivered); err != nil {
		return domain.DeliveryReceipt{}, err
	}
	if alreadyDelivered {
		return domain.DeliveryReceipt{}, domain.ErrUnitDelivered
	}
	var receiptID int64
	if err := tx.QueryRow(ctx, `INSERT INTO manufacturing_deliveries(request_id,execution_id,customer_id,customer_name,actor_id,fingerprint)
VALUES($1::uuid,$2,$3,$4,$5::uuid,$6) RETURNING id`, input.RequestID, executionID, input.CustomerID, customerName, actorID, fingerprint).Scan(&receiptID); err != nil {
		return domain.DeliveryReceipt{}, err
	}
	for _, unitID := range input.UnitIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_delivery_units(delivery_id,execution_id,unit_id) VALUES($1,$2,$3)`, receiptID, executionID, unitID); err != nil {
			return domain.DeliveryReceipt{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE manufacturing_executions SET updated_at=now() WHERE id=$1`, executionID); err != nil {
		return domain.DeliveryReceipt{}, err
	}
	receipt, err := scanDelivery(tx.QueryRow(ctx, selectDelivery+` WHERE d.id=$1`, receiptID))
	if err != nil {
		return domain.DeliveryReceipt{}, err
	}
	return receipt, commit()
}
