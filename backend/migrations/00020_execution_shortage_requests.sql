-- +goose Up
CREATE TABLE manufacturing_execution_shortage_requests (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    execution_id BIGINT NOT NULL UNIQUE REFERENCES manufacturing_executions(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'completed', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO manufacturing_execution_shortage_requests (execution_id)
SELECT DISTINCT e.id
FROM manufacturing_executions e
JOIN manufacturing_execution_materials m ON m.execution_id = e.id
WHERE e.status IN ('registered', 'in_progress', 'paused')
  AND m.quantity_required > m.quantity_reserved + m.quantity_delivered;

-- Reservation rebalancing is the authoritative point where new shortages
-- appear. The unique execution key guarantees one request despite retries.
-- +goose StatementBegin
CREATE FUNCTION create_execution_shortage_request()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.quantity_required > NEW.quantity_reserved + NEW.quantity_delivered
       AND EXISTS (
           SELECT 1 FROM manufacturing_executions e
           WHERE e.id = NEW.execution_id
             AND e.status IN ('registered', 'in_progress', 'paused')
       ) THEN
        INSERT INTO manufacturing_execution_shortage_requests (execution_id)
        VALUES (NEW.execution_id)
        ON CONFLICT (execution_id) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER manufacturing_execution_shortage_request_after_rebalance
AFTER UPDATE OF quantity_required, quantity_reserved, quantity_delivered
ON manufacturing_execution_materials
FOR EACH ROW
EXECUTE FUNCTION create_execution_shortage_request();

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_execution_shortage_requests) THEN
        RAISE EXCEPTION 'cannot roll back execution shortage requests after request history exists';
    END IF;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER manufacturing_execution_shortage_request_after_rebalance ON manufacturing_execution_materials;
DROP FUNCTION create_execution_shortage_request();
DROP TABLE manufacturing_execution_shortage_requests;
