-- +goose Up
CREATE TABLE manufacturing_execution_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    execution_id BIGINT NOT NULL REFERENCES manufacturing_executions(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN (
        'registered','started','paused','resumed','completed','cancelled',
        'shortage','purchase','production','delivery','positive','negative'
    )),
    message TEXT NOT NULL CHECK (length(trim(message)) BETWEEN 1 AND 2000),
    actor_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    actor_name TEXT NOT NULL DEFAULT 'System',
    request_id UUID UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((event_type IN ('positive','negative')) = (request_id IS NOT NULL)),
    CHECK (event_type NOT IN ('positive','negative') OR actor_id IS NOT NULL)
);
CREATE INDEX manufacturing_execution_events_timeline_idx
    ON manufacturing_execution_events(execution_id, occurred_at DESC, id DESC);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_execution_events) THEN
        RAISE EXCEPTION 'cannot roll back execution events while event history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE manufacturing_execution_events;
