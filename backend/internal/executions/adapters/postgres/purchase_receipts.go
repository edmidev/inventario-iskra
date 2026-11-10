package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	purchasingapp "github.com/edgar/inventario/backend/internal/purchasing/app"
	"github.com/jackc/pgx/v5"
)

// LockReceiptItems keeps multi-item receipts in the same lock order as
// execution registration while preserving the receipt's original line order.
func (r *Repo) LockReceiptItems(ctx context.Context, itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	sort.Strings(itemIDs)
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return err
	}
	defer rollback()
	previous := ""
	for _, itemID := range itemIDs {
		if itemID == previous {
			continue
		}
		previous = itemID
		var locked string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM items WHERE id=$1::uuid FOR UPDATE`, itemID).Scan(&locked); err != nil {
			return err
		}
	}
	return commit()
}

// PrepareReceipt serializes a linked receipt on its inventory item before
// stock-in can rebalance reservations. An unlinked line returns an empty
// reservation but still follows the same item-lock order.
func (r *Repo) PrepareReceipt(ctx context.Context, lineID int64, itemID string, alreadyReceived quantity.Quantity) (purchasingapp.PurchaseReservation, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return purchasingapp.PurchaseReservation{}, err
	}
	defer rollback()
	var locked string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM items WHERE id=$1::uuid FOR UPDATE`, itemID).Scan(&locked); err != nil {
		return purchasingapp.PurchaseReservation{}, err
	}
	var snapshot purchasingapp.PurchaseReservation
	var executionStatus, requestStatus string
	var linked quantity.Quantity
	err = tx.QueryRow(ctx, `SELECT r.execution_id,e.status,r.status,m.quantity_reserved,link.quantity
FROM manufacturing_execution_purchase_links link
JOIN manufacturing_execution_shortage_requests r ON r.id=link.shortage_request_id
JOIN manufacturing_executions e ON e.id=r.execution_id
JOIN manufacturing_execution_materials m ON m.execution_id=e.id
JOIN purchase_order_lines l ON l.id=link.purchase_order_line_id AND l.item_id=m.item_id
WHERE link.purchase_order_line_id=$1 AND m.item_id=$2::uuid`, lineID, itemID).
		Scan(&snapshot.ExecutionID, &executionStatus, &requestStatus, &snapshot.ReservedBefore, &linked)
	if errors.Is(err, pgx.ErrNoRows) {
		return purchasingapp.PurchaseReservation{}, commit()
	}
	if err != nil {
		return purchasingapp.PurchaseReservation{}, err
	}
	if executionStatus != "registered" && executionStatus != "in_progress" && executionStatus != "paused" || requestStatus != "open" || linked <= alreadyReceived {
		return purchasingapp.PurchaseReservation{}, commit()
	}
	snapshot.ItemID = itemID
	snapshot.LinkRemaining = linked - alreadyReceived
	return snapshot, commit()
}

// AllocateReceipt moves only this receipt's newly available reservation to
// its linked execution. FIFO remains authoritative for all other stock.
func (r *Repo) AllocateReceipt(ctx context.Context, snapshot purchasingapp.PurchaseReservation, received quantity.Quantity) error {
	if snapshot.ExecutionID == 0 || received <= 0 {
		return nil
	}
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return err
	}
	defer rollback()
	var current, required, delivered quantity.Quantity
	var executionStatus string
	err = tx.QueryRow(ctx, `SELECT m.quantity_reserved,m.quantity_required,m.quantity_delivered,e.status
FROM manufacturing_execution_materials m JOIN manufacturing_executions e ON e.id=m.execution_id
WHERE m.execution_id=$1 AND m.item_id=$2::uuid`, snapshot.ExecutionID, snapshot.ItemID).
		Scan(&current, &required, &delivered, &executionStatus)
	if err != nil {
		return err
	}
	if executionStatus != "registered" && executionStatus != "in_progress" && executionStatus != "paused" {
		return commit()
	}
	desired := minQuantity(received, snapshot.LinkRemaining, required-delivered-snapshot.ReservedBefore)
	// FIFO rebalance may also remove a previous earmarked reservation for this
	// execution. Restore that reservation before adding this receipt's share.
	transfer := snapshot.ReservedBefore + desired - current
	if transfer <= 0 {
		return commit()
	}
	transfer = minQuantity(transfer, required-delivered-current)
	rows, err := tx.Query(ctx, `SELECT m.execution_id,m.quantity_reserved
FROM manufacturing_execution_materials m JOIN manufacturing_executions e ON e.id=m.execution_id
WHERE m.item_id=$1::uuid AND m.execution_id<>$2 AND m.quantity_reserved>0
  AND e.status IN ('registered','in_progress','paused')
ORDER BY e.created_at DESC,e.id DESC FOR UPDATE OF m`, snapshot.ItemID, snapshot.ExecutionID)
	if err != nil {
		return err
	}
	type donor struct {
		id       int64
		reserved quantity.Quantity
	}
	var donors []donor
	for rows.Next() {
		var d donor
		if err := rows.Scan(&d.id, &d.reserved); err != nil {
			rows.Close()
			return err
		}
		donors = append(donors, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	remaining := transfer
	for _, d := range donors {
		take := minQuantity(remaining, d.reserved)
		if _, err := tx.Exec(ctx, `UPDATE manufacturing_execution_materials SET quantity_reserved=quantity_reserved-$3
WHERE execution_id=$1 AND item_id=$2::uuid`, d.id, snapshot.ItemID, take); err != nil {
			return err
		}
		remaining -= take
		if remaining == 0 {
			break
		}
	}
	if remaining != 0 {
		return fmt.Errorf("earmark receipt: %v of %v could not be transferred", remaining, transfer)
	}
	if _, err := tx.Exec(ctx, `UPDATE manufacturing_execution_materials SET quantity_reserved=quantity_reserved+$3
WHERE execution_id=$1 AND item_id=$2::uuid`, snapshot.ExecutionID, snapshot.ItemID, transfer); err != nil {
		return err
	}
	return commit()
}

func minQuantity(first quantity.Quantity, rest ...quantity.Quantity) quantity.Quantity {
	for _, value := range rest {
		if value < first {
			first = value
		}
	}
	return first
}
