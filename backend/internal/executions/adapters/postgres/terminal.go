package postgres

import (
	"context"
	"errors"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5"
)

// Material additions and terminal transitions share this lock before taking
// item locks. It freezes the execution's material set while terminal release
// locks those items in the same order as stock recording.
const executionMaterialLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended('execution-materials:' || $1::bigint::text,0))`

func (r *Repo) Close(ctx context.Context, id int64, to domain.Status) (domain.Execution, error) {
	return r.closeExecution(ctx, id, nil, to)
}

func (r *Repo) closeExecution(ctx context.Context, id int64, expected *domain.Status, to domain.Status) (domain.Execution, error) {
	if id < 1 || (to != domain.StatusCompleted && to != domain.StatusCancelled) {
		return domain.Execution{}, domain.ErrInvalidState
	}
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.Execution{}, err
	}
	defer rollback()
	if _, err := tx.Exec(ctx, executionMaterialLockSQL, id); err != nil {
		return domain.Execution{}, err
	}
	var itemIDs []string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(array_agg(item_id::text ORDER BY item_id),ARRAY[]::text[])
FROM manufacturing_execution_materials WHERE execution_id=$1`, id).Scan(&itemIDs); err != nil {
		return domain.Execution{}, err
	}
	for _, itemID := range itemIDs {
		var lockedID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM items WHERE id=$1::uuid FOR UPDATE`, itemID).Scan(&lockedID); err != nil {
			return domain.Execution{}, err
		}
	}
	var current string
	var planned int
	if err := tx.QueryRow(ctx, `SELECT status,units FROM manufacturing_executions WHERE id=$1 FOR UPDATE`, id).Scan(&current, &planned); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Execution{}, domain.ErrNotFound
		}
		return domain.Execution{}, err
	}
	if expected != nil && current != string(*expected) {
		return domain.Execution{}, domain.ErrInvalidState
	}
	state := domain.Execution{Status: domain.Status(current), Units: planned}
	if to == domain.StatusCompleted {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM finished_product_units WHERE execution_id=$1`, id).Scan(&state.ProducedUnits); err != nil {
			return domain.Execution{}, err
		}
		if err := state.Complete(); err != nil {
			return domain.Execution{}, err
		}
	} else if err := state.Cancel(); err != nil {
		return domain.Execution{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE manufacturing_executions SET status=$2,updated_at=now() WHERE id=$1`, id, string(to)); err != nil {
		return domain.Execution{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE manufacturing_execution_shortage_requests SET status=$2,updated_at=now() WHERE execution_id=$1`, id, string(to)); err != nil {
		return domain.Execution{}, err
	}
	for _, itemID := range itemIDs {
		if _, err := tx.Exec(ctx, `SELECT rebalance_manufacturing_execution_materials($1::uuid)`, itemID); err != nil {
			return domain.Execution{}, err
		}
	}
	execution, err := scanExecution(tx.QueryRow(ctx, selectExecution+` WHERE e.id=$1`, id))
	if err != nil {
		return domain.Execution{}, err
	}
	if err := commit(); err != nil {
		return domain.Execution{}, err
	}
	return execution, nil
}
