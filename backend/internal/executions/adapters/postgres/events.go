package postgres

import (
	"context"
	"errors"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/db"
	"github.com/jackc/pgx/v5"
)

const selectEvent = `SELECT id,execution_id,event_type,message,COALESCE(actor_id::text,''),actor_name,occurred_at FROM manufacturing_execution_events`

func insertAutomaticEvent(ctx context.Context, tx pgx.Tx, executionID int64, actorID, eventType, message string) error {
	_, err := tx.Exec(ctx, `INSERT INTO manufacturing_execution_events(execution_id,event_type,message,actor_id,actor_name)
VALUES($1,$2,$3,$4::uuid,(SELECT name FROM users WHERE id=$4::uuid))`, executionID, eventType, message, actorID)
	return err
}

func scanEvent(row pgx.Row) (domain.ExecutionEvent, error) {
	var event domain.ExecutionEvent
	err := row.Scan(&event.ID, &event.ExecutionID, &event.Type, &event.Message, &event.ActorID, &event.ActorName, &event.OccurredAt)
	return event, err
}

func (r *Repo) RecordIncident(ctx context.Context, executionID int64, actorID string, input domain.IncidentInput) (domain.ExecutionEvent, error) {
	tx, commit, rollback, err := db.Begin(ctx, r.pool)
	if err != nil {
		return domain.ExecutionEvent{}, err
	}
	defer rollback()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('execution-incident:' || $1::text,0))`, input.RequestID); err != nil {
		return domain.ExecutionEvent{}, err
	}
	var priorExecutionID int64
	var priorType, priorMessage, priorActor string
	err = tx.QueryRow(ctx, `SELECT execution_id,event_type,message,actor_id::text FROM manufacturing_execution_events WHERE request_id=$1::uuid`, input.RequestID).Scan(&priorExecutionID, &priorType, &priorMessage, &priorActor)
	if err == nil {
		if priorExecutionID != executionID || priorType != input.Type || priorMessage != input.Message || priorActor != actorID {
			return domain.ExecutionEvent{}, domain.ErrIncidentReplay
		}
		event, err := scanEvent(tx.QueryRow(ctx, selectEvent+` WHERE request_id=$1::uuid`, input.RequestID))
		if err != nil {
			return domain.ExecutionEvent{}, err
		}
		return event, commit()
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ExecutionEvent{}, err
	}
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id FROM manufacturing_executions WHERE id=$1 FOR KEY SHARE`, executionID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ExecutionEvent{}, domain.ErrNotFound
		}
		return domain.ExecutionEvent{}, err
	}
	var actorName string
	if err := tx.QueryRow(ctx, `SELECT name FROM users WHERE id=$1::uuid`, actorID).Scan(&actorName); err != nil {
		return domain.ExecutionEvent{}, err
	}
	event, err := scanEvent(tx.QueryRow(ctx, `INSERT INTO manufacturing_execution_events(execution_id,event_type,message,actor_id,actor_name,request_id)
VALUES($1,$2,$3,$4::uuid,$5,$6::uuid) RETURNING id,execution_id,event_type,message,actor_id::text,actor_name,occurred_at`, executionID, input.Type, input.Message, actorID, actorName, input.RequestID))
	if err != nil {
		return domain.ExecutionEvent{}, err
	}
	return event, commit()
}

func (r *Repo) ListEvents(ctx context.Context, executionID int64) ([]domain.ExecutionEvent, error) {
	var id int64
	if err := r.pool.QueryRow(ctx, `SELECT id FROM manufacturing_executions WHERE id=$1`, executionID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	rows, err := r.pool.Query(ctx, selectEvent+` WHERE execution_id=$1 ORDER BY occurred_at,id`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []domain.ExecutionEvent{}
	for rows.Next() {
		var event domain.ExecutionEvent
		if err := rows.Scan(&event.ID, &event.ExecutionID, &event.Type, &event.Message, &event.ActorID, &event.ActorName, &event.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}
