-- +goose Up
CREATE TABLE manufacturing_execution_material_additions (
    request_id UUID PRIMARY KEY,
    execution_id BIGINT NOT NULL,
    item_id UUID NOT NULL,
    quantity NUMERIC(18,3) NOT NULL CHECK (quantity > 0),
    fingerprint TEXT NOT NULL CHECK (length(fingerprint) = 64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (execution_id, item_id)
        REFERENCES manufacturing_execution_materials(execution_id, item_id) ON DELETE RESTRICT
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_execution_material_additions) THEN
        RAISE EXCEPTION 'cannot roll back supplemental material additions while request history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE manufacturing_execution_material_additions;
