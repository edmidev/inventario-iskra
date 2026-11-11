-- +goose Up
CREATE TABLE manufacturing_execution_materials (
    execution_id BIGINT NOT NULL REFERENCES manufacturing_executions(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id) ON DELETE RESTRICT,
    quantity_required NUMERIC(18,3) NOT NULL CHECK (quantity_required > 0),
    quantity_reserved NUMERIC(18,3) NOT NULL DEFAULT 0 CHECK (quantity_reserved >= 0),
    quantity_delivered NUMERIC(18,3) NOT NULL DEFAULT 0 CHECK (quantity_delivered >= 0),
    quantity_returned NUMERIC(18,3) NOT NULL DEFAULT 0 CHECK (quantity_returned >= 0 AND quantity_returned <= quantity_delivered),
    PRIMARY KEY (execution_id, item_id),
    CHECK (quantity_reserved + quantity_delivered <= quantity_required)
);

CREATE INDEX manufacturing_execution_materials_item_idx
    ON manufacturing_execution_materials(item_id, execution_id);

CREATE TABLE manufacturing_execution_material_operations (
    request_id UUID PRIMARY KEY,
    execution_id BIGINT NOT NULL,
    item_id UUID NOT NULL,
    movement_id BIGINT NOT NULL UNIQUE REFERENCES stock_movements(id) ON DELETE RESTRICT,
    fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (execution_id, item_id) REFERENCES manufacturing_execution_materials(execution_id, item_id) ON DELETE RESTRICT
);

-- Callers must hold the corresponding items row lock. Recomputing instead of
-- incrementally mutating reservations makes stock corrections deterministic.
-- +goose StatementBegin
CREATE FUNCTION rebalance_manufacturing_execution_materials(p_item_id UUID)
RETURNS VOID AS $$
BEGIN
    WITH active_needs AS (
        SELECT m.execution_id, GREATEST(m.quantity_required - m.quantity_delivered, 0) AS needed,
               e.created_at, i.stock_current,
               SUM(GREATEST(m.quantity_required - m.quantity_delivered, 0)) OVER (
                   ORDER BY e.created_at, e.id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) AS cumulative_needed
        FROM manufacturing_execution_materials m
        JOIN manufacturing_executions e ON e.id = m.execution_id
        JOIN items i ON i.id = m.item_id
        WHERE m.item_id = p_item_id AND e.status IN ('registered', 'in_progress', 'paused')
    ), allocated AS (
        SELECT execution_id, GREATEST(0, LEAST(needed, stock_current - (cumulative_needed - needed))) AS reserved
        FROM active_needs
    )
    UPDATE manufacturing_execution_materials m
    SET quantity_reserved = COALESCE(a.reserved, 0)
    FROM (SELECT execution_id, reserved FROM allocated) a
    WHERE m.item_id = p_item_id AND m.execution_id = a.execution_id;

    UPDATE manufacturing_execution_materials m SET quantity_reserved = 0
    WHERE m.item_id = p_item_id AND NOT EXISTS (
        SELECT 1 FROM manufacturing_executions e
        WHERE e.id = m.execution_id AND e.status IN ('registered', 'in_progress', 'paused'));
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Preserve T4 executions created before the reservation model. Existing
-- physical stock is assigned to the oldest active execution first.
WITH needs AS (
    SELECT e.id AS execution_id,
           (m.value->>'item_id')::UUID AS item_id,
           SUM((m.value->>'quantity_per_unit')::NUMERIC * e.units) AS quantity_required
    FROM manufacturing_executions e
    CROSS JOIN LATERAL jsonb_array_elements(COALESCE(e.project_snapshot->'consumables', '[]'::JSONB)) AS m(value)
    GROUP BY e.id, e.units, (m.value->>'item_id')::UUID
)
INSERT INTO manufacturing_execution_materials(execution_id, item_id, quantity_required)
SELECT execution_id, item_id, quantity_required FROM needs;

-- Initial allocation for any execution history already present.
-- +goose StatementBegin
DO $$
DECLARE v_item UUID;
BEGIN
    FOR v_item IN SELECT DISTINCT item_id FROM manufacturing_execution_materials ORDER BY item_id LOOP
        PERFORM rebalance_manufacturing_execution_materials(v_item);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_executions) THEN
        RAISE EXCEPTION 'cannot roll back material reservations while manufacturing execution history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP FUNCTION rebalance_manufacturing_execution_materials(UUID);
DROP TABLE manufacturing_execution_material_operations;
DROP TABLE manufacturing_execution_materials;
