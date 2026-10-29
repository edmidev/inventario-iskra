-- +goose Up
CREATE TABLE suppliers (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name         TEXT NOT NULL,
    contact_name TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL DEFAULT '',
    phone        TEXT NOT NULL DEFAULT '',
    notes        TEXT NOT NULL DEFAULT '',
    active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ
);

-- A name is unique among live suppliers only, so a deleted name can be reused.
CREATE UNIQUE INDEX suppliers_name_live_uq ON suppliers (lower(name)) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE suppliers;
