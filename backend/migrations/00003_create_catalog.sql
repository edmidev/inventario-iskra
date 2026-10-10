-- +goose Up
-- Categories: case-insensitive unique name, constrained code prefix.
ALTER TABLE categories DROP CONSTRAINT categories_name_key;
CREATE UNIQUE INDEX categories_name_lower_key ON categories (lower(name));
ALTER TABLE categories ADD CONSTRAINT categories_code_prefix_format CHECK (code_prefix ~ '^[A-Z0-9]{2,8}$');
ALTER TABLE categories ADD CONSTRAINT categories_name_not_blank CHECK (length(btrim(name)) > 0);

-- Locations: a parent can no longer disappear underneath its children.
-- (Cycle prevention is enforced by the application.)
ALTER TABLE locations DROP CONSTRAINT locations_parent_id_fkey;
ALTER TABLE locations ADD CONSTRAINT locations_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES locations (id) ON DELETE RESTRICT;
ALTER TABLE locations ADD CONSTRAINT locations_not_own_parent CHECK (parent_id IS NULL OR parent_id <> id);
ALTER TABLE locations ADD CONSTRAINT locations_name_not_blank CHECK (length(btrim(name)) > 0);

CREATE TABLE items (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                    TEXT NOT NULL UNIQUE,
    description             TEXT NOT NULL CHECK (length(btrim(description)) > 0),
    category_id             BIGINT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    kind                    TEXT NOT NULL CHECK (kind IN ('asset', 'consumable')),
    status                  TEXT NOT NULL DEFAULT 'functional' CHECK (status IN ('functional', 'inoperative', 'in_repair')),
    unit                    TEXT,
    stock_current           INTEGER NOT NULL DEFAULT 1 CHECK (stock_current >= 0),
    stock_min               INTEGER NOT NULL DEFAULT 0 CHECK (stock_min >= 0),
    location_id             BIGINT REFERENCES locations (id) ON DELETE RESTRICT,
    last_maintenance_at     TIMESTAMPTZ,
    maintenance_interval_days INTEGER CHECK (maintenance_interval_days IS NULL OR maintenance_interval_days > 0),
    attributes              JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    notes                   TEXT NOT NULL DEFAULT '',
    deleted_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT items_kind_rules CHECK (
        (kind = 'asset' AND unit IS NULL AND stock_current = 1 AND stock_min = 0)
        OR (kind = 'consumable' AND unit IS NOT NULL AND length(btrim(unit)) > 0)
    )
);

CREATE INDEX items_category_id_idx ON items (category_id) WHERE deleted_at IS NULL;
CREATE INDEX items_location_id_idx ON items (location_id) WHERE deleted_at IS NULL;
CREATE INDEX items_status_idx ON items (status) WHERE deleted_at IS NULL;
CREATE INDEX items_kind_idx ON items (kind) WHERE deleted_at IS NULL;
CREATE INDEX items_updated_at_idx ON items (updated_at DESC) WHERE deleted_at IS NULL;

-- Atomic per-prefix counter used to generate item codes.
CREATE TABLE code_sequences (
    prefix     TEXT PRIMARY KEY,
    last_value INTEGER NOT NULL DEFAULT 0
);

-- Trigram indexes speed up ILIKE search when pg_trgm is available; search
-- still works (sequential scan) when the extension cannot be installed.
-- +goose StatementBegin
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
    CREATE INDEX items_description_trgm_idx ON items USING gin (description gin_trgm_ops);
    CREATE INDEX items_code_trgm_idx ON items USING gin (code gin_trgm_ops);
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_trgm unavailable (%), search falls back to ILIKE scans', SQLERRM;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE code_sequences;
DROP TABLE items;
ALTER TABLE locations DROP CONSTRAINT locations_name_not_blank;
ALTER TABLE locations DROP CONSTRAINT locations_not_own_parent;
ALTER TABLE locations DROP CONSTRAINT locations_parent_id_fkey;
ALTER TABLE locations ADD CONSTRAINT locations_parent_id_fkey
    FOREIGN KEY (parent_id) REFERENCES locations (id) ON DELETE SET NULL;
ALTER TABLE categories DROP CONSTRAINT categories_name_not_blank;
ALTER TABLE categories DROP CONSTRAINT categories_code_prefix_format;
DROP INDEX categories_name_lower_key;
ALTER TABLE categories ADD CONSTRAINT categories_name_key UNIQUE (name);
