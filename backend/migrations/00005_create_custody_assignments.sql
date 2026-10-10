-- +goose Up
-- One row per loan of an asset to a person or container (a `locations` row).
-- While returned_at IS NULL the loan is active; the partial unique index
-- guarantees at most one active loan per item even under concurrent requests.
-- Origin and return locations use SET NULL so deleting a location never
-- erases loan history; the assignee is RESTRICTed because history needs it.
CREATE TABLE custody_assignments (
    id                      BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    item_id                 UUID NOT NULL REFERENCES items (id) ON DELETE RESTRICT,
    assignee_location_id    BIGINT NOT NULL REFERENCES locations (id) ON DELETE RESTRICT,
    origin_location_id      BIGINT REFERENCES locations (id) ON DELETE SET NULL,
    assigned_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    due_at                  TIMESTAMPTZ,
    returned_at             TIMESTAMPTZ,
    returned_to_location_id BIGINT REFERENCES locations (id) ON DELETE SET NULL,
    assigned_by             UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    returned_by             UUID REFERENCES users (id) ON DELETE RESTRICT,
    assign_note             TEXT NOT NULL DEFAULT '' CHECK (length(assign_note) <= 500),
    return_note             TEXT NOT NULL DEFAULT '' CHECK (length(return_note) <= 500),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT custody_assignments_due_after_assigned CHECK (due_at IS NULL OR due_at > assigned_at),
    CONSTRAINT custody_assignments_returned_after_assigned CHECK (returned_at IS NULL OR returned_at >= assigned_at),
    CONSTRAINT custody_assignments_return_shape CHECK (
        (returned_at IS NULL AND returned_by IS NULL AND returned_to_location_id IS NULL AND return_note = '')
        OR (returned_at IS NOT NULL AND returned_by IS NOT NULL)
    )
);

CREATE UNIQUE INDEX custody_assignments_one_active_per_item
    ON custody_assignments (item_id) WHERE returned_at IS NULL;
CREATE INDEX custody_assignments_item_history_idx
    ON custody_assignments (item_id, assigned_at DESC, id DESC);
CREATE INDEX custody_assignments_active_idx
    ON custody_assignments (assigned_at, id) WHERE returned_at IS NULL;
CREATE INDEX custody_assignments_active_due_idx
    ON custody_assignments (due_at) WHERE returned_at IS NULL AND due_at IS NOT NULL;
CREATE INDEX custody_assignments_active_assignee_idx
    ON custody_assignments (assignee_location_id) WHERE returned_at IS NULL;

-- +goose Down
DROP TABLE custody_assignments;
