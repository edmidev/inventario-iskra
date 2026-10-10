-- +goose Up
CREATE TABLE categories (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    code_prefix TEXT NOT NULL UNIQUE
);

CREATE TABLE locations (
    id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL CHECK (kind IN ('storage', 'container', 'person')),
    parent_id BIGINT REFERENCES locations (id) ON DELETE SET NULL
);

CREATE INDEX locations_parent_id_idx ON locations (parent_id);

-- +goose Down
DROP TABLE locations;
DROP TABLE categories;
