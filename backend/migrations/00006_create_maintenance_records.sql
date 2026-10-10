-- +goose Up
-- One row per maintenance event of an asset.
--   corrective: a failure report. It starts open (the asset is broken or in
--               repair) and is closed when the repair finishes.
--   preventive: a scheduled service already performed; always created closed.
-- reported_at is the failure date (corrective) or the date performed
-- (preventive). The partial unique index guarantees at most one open
-- failure per item even under concurrent reports. Rows are never deleted.
CREATE TABLE maintenance_records (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    item_id         UUID NOT NULL REFERENCES items (id) ON DELETE RESTRICT,
    kind            TEXT NOT NULL CHECK (kind IN ('preventive', 'corrective')),
    status          TEXT NOT NULL CHECK (status IN ('open', 'closed')),
    reported_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    description     TEXT NOT NULL CHECK (length(btrim(description)) > 0 AND length(description) <= 1000),
    destination     TEXT CHECK (destination IS NULL OR (length(btrim(destination)) > 0 AND length(destination) <= 120)),
    cost            NUMERIC(12, 2) CHECK (cost IS NULL OR cost >= 0),
    next_due_at     TIMESTAMPTZ,
    closed_at       TIMESTAMPTZ,
    resolution_note TEXT CHECK (resolution_note IS NULL OR length(resolution_note) <= 1000),
    reported_by     UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    closed_by       UUID REFERENCES users (id) ON DELETE RESTRICT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT maintenance_records_preventive_closed CHECK (kind = 'corrective' OR status = 'closed'),
    CONSTRAINT maintenance_records_open_shape CHECK (
        status <> 'open'
        OR (closed_at IS NULL AND closed_by IS NULL AND resolution_note IS NULL AND cost IS NULL AND next_due_at IS NULL)
    ),
    CONSTRAINT maintenance_records_closed_shape CHECK (
        status <> 'closed' OR (closed_at IS NOT NULL AND closed_by IS NOT NULL AND closed_at >= reported_at)
    ),
    CONSTRAINT maintenance_records_next_due_rules CHECK (
        next_due_at IS NULL OR (kind = 'preventive' AND next_due_at > reported_at)
    )
);

CREATE UNIQUE INDEX maintenance_records_one_open_failure_per_item
    ON maintenance_records (item_id) WHERE kind = 'corrective' AND status = 'open';
CREATE INDEX maintenance_records_item_history_idx
    ON maintenance_records (item_id, reported_at DESC, id DESC);
CREATE INDEX maintenance_records_open_idx
    ON maintenance_records (reported_at, id) WHERE status = 'open';
CREATE INDEX maintenance_records_destination_idx
    ON maintenance_records (lower(destination)) WHERE destination IS NOT NULL;

-- +goose Down
DROP TABLE maintenance_records;
