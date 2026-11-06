-- +goose Up
CREATE TABLE customers (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120),
    identification TEXT NOT NULL DEFAULT '',
    contact TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX customers_name_live_uq ON customers(lower(name)) WHERE deleted_at IS NULL;
-- Each template's complete bill of materials is updated atomically; execution
-- snapshots will own independent copies, never references to these mutable lists.
CREATE TABLE project_templates (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name TEXT NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120),
    description TEXT NOT NULL DEFAULT '',
    characteristics JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(characteristics)='array'),
    consumables JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(consumables)='array'),
    tools JSONB NOT NULL DEFAULT '[]' CHECK(jsonb_typeof(tools)='array'),
    estimated_days INTEGER NOT NULL CHECK(estimated_days BETWEEN 1 AND 36500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX project_templates_name_live_uq ON project_templates(lower(name)) WHERE deleted_at IS NULL;

-- +goose Down
DROP TABLE project_templates;
DROP TABLE customers;
