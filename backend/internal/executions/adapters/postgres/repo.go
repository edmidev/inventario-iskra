// Package postgres persists manufacturing execution snapshots and assignments.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
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
ARRAY(SELECT user_id::text FROM manufacturing_execution_workers w WHERE w.execution_id=e.id ORDER BY user_id)::TEXT[],
COALESCE((SELECT id FROM manufacturing_execution_shortage_requests r WHERE r.execution_id=e.id),0),
COALESCE((SELECT jsonb_agg(jsonb_build_object(
 'item_id',m.item_id::text,'required_quantity',m.quantity_required,
 'reserved_quantity',m.quantity_reserved,'delivered_quantity',m.quantity_delivered,
 'returned_quantity',m.quantity_returned,
 'deficit_quantity',m.quantity_required-m.quantity_reserved-m.quantity_delivered,
 'purchased_pending_quantity',p.pending,
 'unpurchased_quantity',GREATEST(m.quantity_required-m.quantity_reserved-m.quantity_delivered-p.pending,0))
 ORDER BY m.item_id)
 FROM manufacturing_execution_materials m
 LEFT JOIN LATERAL (
   SELECT COALESCE(SUM(GREATEST(link.quantity-l.received_qty,0)),0) AS pending
   FROM manufacturing_execution_purchase_links link
   JOIN manufacturing_execution_shortage_requests req ON req.id=link.shortage_request_id
   JOIN purchase_order_lines l ON l.id=link.purchase_order_line_id
   JOIN purchase_orders po ON po.id=l.order_id
   WHERE req.execution_id=m.execution_id AND req.status='open'
     AND l.item_id=m.item_id AND po.status<>'cancelled'
 ) p ON TRUE
 WHERE m.execution_id=e.id),'[]'::JSONB)
,
COALESCE((SELECT array_agg(t.item_id::text ORDER BY t.item_id)
 FROM manufacturing_execution_supplemental_tools t WHERE t.execution_id=e.id),ARRAY[]::TEXT[])
FROM manufacturing_executions e`

func scanExecution(row pgx.Row) (domain.Execution, error) {
	var e domain.Execution
	var snapshot []byte
	var status string
	var materials []byte
	var supplementalTools []string
	err := row.Scan(&e.ID, &snapshot, &e.Units, &e.EstimatedDays, &status, &e.PrincipalUserID, &e.CreatedAt, &e.UpdatedAt, &e.CustomerIDs, &e.WorkerIDs, &e.ShortageRequestID, &materials, &supplementalTools)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Execution{}, domain.ErrNotFound
		}
		return domain.Execution{}, err
	}
	if err := json.Unmarshal(snapshot, &e.ProjectSnapshot); err != nil {
		return domain.Execution{}, err
	}
	if err := json.Unmarshal(materials, &e.Materials); err != nil {
		return domain.Execution{}, err
	}
	e.Tools = mergeToolIDs(e.Tools, supplementalTools)
	e.Status = domain.Status(status)
	return e, nil
}

func mergeToolIDs(templateTools, additionalTools []string) []string {
	merged := append([]string{}, templateTools...)
	seen := make(map[string]struct{}, len(templateTools)+len(additionalTools))
	for _, id := range templateTools {
		seen[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	}
	for _, id := range additionalTools {
		key := strings.ToLower(strings.TrimSpace(id))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, id)
	}
	return merged
}

func (r *Repo) Create(ctx context.Context, e domain.Execution) (domain.Execution, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rollback()
	itemIDs := make([]string, 0, len(e.Consumables)+len(e.AdditionalMaterials)+len(e.AdditionalTools))
	materialIDs := make([]string, 0, len(e.Consumables)+len(e.AdditionalMaterials))
	required := make(map[string]quantity.Quantity, len(e.Consumables))
	for _, material := range e.Consumables {
		amount, scaleErr := domain.RequiredMaterialQuantity(material.QuantityPerUnit, e.Units)
		if scaleErr != nil {
			return domain.Execution{}, scaleErr
		}
		if err := addRequiredQuantity(required, material.ItemID, amount); err != nil {
			return domain.Execution{}, err
		}
	}
	for _, material := range e.AdditionalMaterials {
		if err := addRequiredQuantity(required, material.ItemID, material.Quantity); err != nil {
			return domain.Execution{}, err
		}
	}
	for itemID := range required {
		itemIDs = append(itemIDs, itemID)
		materialIDs = append(materialIDs, itemID)
	}
	sort.Strings(materialIDs)
	toolIDs := make(map[string]struct{}, len(e.AdditionalTools))
	for _, itemID := range e.AdditionalTools {
		if _, exists := toolIDs[itemID]; exists {
			continue
		}
		toolIDs[itemID] = struct{}{}
		if _, exists := required[itemID]; !exists {
			itemIDs = append(itemIDs, itemID)
		}
	}
	sort.Strings(itemIDs)
	for _, itemID := range itemIDs {
		var kind string
		lock := `FOR KEY SHARE`
		if _, isMaterial := required[itemID]; isMaterial {
			lock = `FOR UPDATE`
		}
		if err := tx.QueryRow(ctx, `SELECT kind FROM items WHERE id=$1::uuid AND deleted_at IS NULL `+lock, itemID).Scan(&kind); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if _, isMaterial := required[itemID]; isMaterial {
					return domain.Execution{}, domain.ErrMaterialUnavailable
				}
				return domain.Execution{}, domain.ErrToolUnavailable
			}
			return domain.Execution{}, err
		}
		_, isMaterial := required[itemID]
		_, isTool := toolIDs[itemID]
		if isMaterial {
			if kind != "consumable" {
				return domain.Execution{}, domain.ErrMaterialUnavailable
			}
		}
		if isTool && kind != "asset" {
			return domain.Execution{}, domain.ErrToolUnavailable
		}
	}
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
	for itemID := range toolIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO manufacturing_execution_supplemental_tools(execution_id,item_id)
VALUES($1,$2::uuid) ON CONFLICT (execution_id,item_id) DO NOTHING`, e.ID, itemID); err != nil {
			return domain.Execution{}, err
		}
	}
	for _, itemID := range materialIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO manufacturing_execution_materials(execution_id,item_id,quantity_required)
VALUES($1,$2::uuid,$3)`, e.ID, itemID, required[itemID]); err != nil {
			return domain.Execution{}, err
		}
	}
	for _, itemID := range materialIDs {
		if _, err = tx.Exec(ctx, `SELECT rebalance_manufacturing_execution_materials($1::uuid)`, itemID); err != nil {
			return domain.Execution{}, err
		}
	}
	created, err := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, e.ID))
	if err != nil {
		return domain.Execution{}, err
	}
	if err = commit(); err != nil {
		return domain.Execution{}, err
	}
	return created, nil
}

func addRequiredQuantity(required map[string]quantity.Quantity, itemID string, amount quantity.Quantity) error {
	total := required[itemID] + amount
	if total <= 0 {
		return domain.ErrInvalidReservationAllocation
	}
	if _, err := domain.RequiredMaterialQuantity(total, 1); err != nil {
		return domain.ErrInvalidReservationAllocation
	}
	required[itemID] = total
	return nil
}

func (r *Repo) AddMaterialRequirement(ctx context.Context, executionID int64, requestID, itemID string, amount quantity.Quantity, fingerprint string) (domain.Execution, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rollback()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, requestID); err != nil {
		return domain.Execution{}, err
	}
	var kind string
	err = tx.QueryRow(ctx, `SELECT kind FROM items WHERE id=$1::uuid AND deleted_at IS NULL FOR UPDATE`, itemID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, domain.ErrMaterialUnavailable
	}
	if err != nil {
		return domain.Execution{}, err
	}
	if kind != "consumable" {
		return domain.Execution{}, domain.ErrMaterialUnavailable
	}
	var priorFingerprint string
	err = tx.QueryRow(ctx, `SELECT fingerprint FROM manufacturing_execution_material_additions WHERE request_id=$1::uuid`, requestID).Scan(&priorFingerprint)
	if err == nil {
		if priorFingerprint != fingerprint {
			return domain.Execution{}, stockdomain.ErrIdempotencyConflict
		}
		current, scanErr := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, executionID))
		if scanErr != nil {
			return domain.Execution{}, scanErr
		}
		if err = commit(); err != nil {
			return domain.Execution{}, err
		}
		return current, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, err
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM manufacturing_executions WHERE id=$1 FOR UPDATE`, executionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Execution{}, err
	}
	if status != string(domain.StatusRegistered) && status != string(domain.StatusInProgress) && status != string(domain.StatusPaused) {
		return domain.Execution{}, domain.ErrMaterialAdditionClosed
	}
	if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_execution_materials(execution_id,item_id,quantity_required)
VALUES($1,$2::uuid,$3)
ON CONFLICT (execution_id,item_id) DO UPDATE SET quantity_required=manufacturing_execution_materials.quantity_required+EXCLUDED.quantity_required`, executionID, itemID, amount); err != nil {
		return domain.Execution{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_execution_material_additions(request_id,execution_id,item_id,quantity,fingerprint)
VALUES($1::uuid,$2,$3::uuid,$4,$5)`, requestID, executionID, itemID, amount, fingerprint); err != nil {
		return domain.Execution{}, err
	}
	if _, err := tx.Exec(ctx, `SELECT rebalance_manufacturing_execution_materials($1::uuid)`, itemID); err != nil {
		return domain.Execution{}, err
	}
	created, err := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, executionID))
	if err != nil {
		return domain.Execution{}, err
	}
	if err = commit(); err != nil {
		return domain.Execution{}, err
	}
	return created, nil
}

func (r *Repo) AddToolRequirement(ctx context.Context, executionID int64, requestID, userID, itemID, fingerprint string) (domain.Execution, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rollback()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, requestID); err != nil {
		return domain.Execution{}, err
	}
	var kind string
	err = tx.QueryRow(ctx, `SELECT kind FROM items WHERE id=$1::uuid AND deleted_at IS NULL FOR KEY SHARE`, itemID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && kind != "asset") {
		return domain.Execution{}, domain.ErrToolUnavailable
	}
	if err != nil {
		return domain.Execution{}, err
	}
	var priorFingerprint string
	err = tx.QueryRow(ctx, `SELECT fingerprint FROM manufacturing_execution_tool_additions WHERE request_id=$1::uuid`, requestID).Scan(&priorFingerprint)
	if err == nil {
		if priorFingerprint != fingerprint {
			return domain.Execution{}, stockdomain.ErrIdempotencyConflict
		}
		current, scanErr := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, executionID))
		if scanErr != nil {
			return domain.Execution{}, scanErr
		}
		if err = commit(); err != nil {
			return domain.Execution{}, err
		}
		return current, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, err
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM manufacturing_executions WHERE id=$1 FOR UPDATE`, executionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Execution{}, err
	}
	if status != string(domain.StatusRegistered) && status != string(domain.StatusInProgress) && status != string(domain.StatusPaused) {
		return domain.Execution{}, domain.ErrToolAdditionClosed
	}
	if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_execution_supplemental_tools(execution_id,item_id)
VALUES($1,$2::uuid) ON CONFLICT (execution_id,item_id) DO NOTHING`, executionID, itemID); err != nil {
		return domain.Execution{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_execution_tool_additions(request_id,execution_id,item_id,actor_id,fingerprint)
VALUES($1::uuid,$2,$3::uuid,$4::uuid,$5)`, requestID, executionID, itemID, userID, fingerprint); err != nil {
		return domain.Execution{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE manufacturing_executions SET updated_at=now() WHERE id=$1`, executionID); err != nil {
		return domain.Execution{}, err
	}
	created, err := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, executionID))
	if err != nil {
		return domain.Execution{}, err
	}
	if err = commit(); err != nil {
		return domain.Execution{}, err
	}
	return created, nil
}

func (r *Repo) Get(ctx context.Context, id int64) (domain.Execution, error) {
	return scanExecution(r.pool.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, id))
}

// LinkPurchaseOrderLine earmarks an unreceived supplier-order line for an
// execution's current deficit. Repeating an identical link is idempotent.
func (r *Repo) LinkPurchaseOrderLine(ctx context.Context, executionID, lineID int64, amount quantity.Quantity) (domain.Execution, error) {
	if executionID < 1 || lineID < 1 || amount <= 0 {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"quantity": "execution, line and positive quantity are required"}}
	}
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rollback()
	// Purchasing receives under the order row lock before touching stock or
	// execution state. Link in the same order to avoid a cross-module cycle.
	var itemID, orderStatus string
	var lineQuantity, receivedQuantity quantity.Quantity
	err = tx.QueryRow(ctx, `SELECT l.item_id::text,l.quantity,l.received_qty,po.status
FROM purchase_order_lines l JOIN purchase_orders po ON po.id=l.order_id
WHERE l.id=$1 AND l.item_id IS NOT NULL FOR UPDATE OF po,l`, lineID).Scan(&itemID, &lineQuantity, &receivedQuantity, &orderStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"purchase_order_line_id": "must reference an existing consumable line"}}
	}
	if err != nil {
		return domain.Execution{}, err
	}
	if orderStatus != "ordered" || receivedQuantity != 0 || amount > lineQuantity {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"purchase_order_line_id": "line must be ordered, unreceived and have enough quantity"}}
	}
	var requestID int64
	var executionStatus, requestStatus string
	err = tx.QueryRow(ctx, `SELECT r.id,e.status,r.status
FROM manufacturing_executions e
JOIN manufacturing_execution_shortage_requests r ON r.execution_id=e.id
WHERE e.id=$1 FOR UPDATE OF e,r`, executionID).Scan(&requestID, &executionStatus, &requestStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Execution{}, err
	}
	if requestStatus != "open" || (executionStatus != "registered" && executionStatus != "in_progress" && executionStatus != "paused") {
		return domain.Execution{}, domain.ErrInvalidState
	}
	var existingRequest int64
	var existingAmount quantity.Quantity
	err = tx.QueryRow(ctx, `SELECT shortage_request_id,quantity FROM manufacturing_execution_purchase_links WHERE purchase_order_line_id=$1`, lineID).Scan(&existingRequest, &existingAmount)
	if err == nil {
		if existingRequest != requestID || existingAmount != amount {
			return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"purchase_order_line_id": "line is already linked with different terms"}}
		}
		out, err := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, executionID))
		if err != nil {
			return domain.Execution{}, err
		}
		return out, commit()
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, err
	}
	var deficit, pending quantity.Quantity
	err = tx.QueryRow(ctx, `SELECT m.quantity_required-m.quantity_reserved-m.quantity_delivered,
COALESCE((SELECT SUM(GREATEST(link.quantity-l.received_qty,0))
FROM manufacturing_execution_purchase_links link
JOIN purchase_order_lines l ON l.id=link.purchase_order_line_id
JOIN purchase_orders po ON po.id=l.order_id
WHERE link.shortage_request_id=$1 AND l.item_id=m.item_id AND po.status<>'cancelled'),0)
FROM manufacturing_execution_materials m WHERE m.execution_id=$2 AND m.item_id=$3::uuid`, requestID, executionID, itemID).Scan(&deficit, &pending)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"quantity": "exceeds the unpurchased execution shortage"}}
	}
	if err != nil {
		return domain.Execution{}, err
	}
	if amount > deficit-pending {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"quantity": "exceeds the unpurchased execution shortage"}}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO manufacturing_execution_purchase_links(shortage_request_id,purchase_order_line_id,quantity) VALUES($1,$2,$3)`, requestID, lineID, amount); err != nil {
		return domain.Execution{}, err
	}
	out, err := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, executionID))
	if err != nil {
		return domain.Execution{}, err
	}
	return out, commit()
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
	requestStatus := ""
	switch to {
	case domain.StatusCompleted:
		requestStatus = "completed"
	case domain.StatusCancelled:
		requestStatus = "cancelled"
	}
	if requestStatus != "" {
		if _, err := tx.Exec(ctx, `UPDATE manufacturing_execution_shortage_requests SET status=$2,updated_at=now() WHERE execution_id=$1`, id, requestStatus); err != nil {
			return domain.Execution{}, err
		}
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
