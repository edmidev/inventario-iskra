-- +goose Up
-- Delivery recipients are chosen per finished unit, not at production time.
ALTER TABLE finished_product_units ADD CONSTRAINT finished_product_unit_execution_key UNIQUE (id, execution_id);

CREATE TABLE manufacturing_deliveries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number TEXT GENERATED ALWAYS AS ('DEL-' || lpad(id::TEXT, GREATEST(6, length(id::TEXT)), '0')) STORED UNIQUE,
    request_id UUID NOT NULL UNIQUE,
    execution_id BIGINT NOT NULL REFERENCES manufacturing_executions(id) ON DELETE RESTRICT,
    customer_id BIGINT NOT NULL,
    customer_name TEXT NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    fingerprint TEXT NOT NULL,
    delivered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, execution_id),
    FOREIGN KEY (execution_id, customer_id) REFERENCES manufacturing_execution_customers(execution_id, customer_id) ON DELETE RESTRICT
);
CREATE INDEX manufacturing_deliveries_execution_idx ON manufacturing_deliveries(execution_id, id);

CREATE TABLE manufacturing_delivery_units (
    delivery_id BIGINT NOT NULL,
    execution_id BIGINT NOT NULL,
    unit_id BIGINT NOT NULL UNIQUE,
    PRIMARY KEY (delivery_id, unit_id),
    FOREIGN KEY (delivery_id, execution_id) REFERENCES manufacturing_deliveries(id, execution_id) ON DELETE RESTRICT,
    FOREIGN KEY (unit_id, execution_id) REFERENCES finished_product_units(id, execution_id) ON DELETE RESTRICT
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_deliveries) THEN
        RAISE EXCEPTION 'cannot roll back finished-product deliveries while delivery history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE manufacturing_delivery_units;
DROP TABLE manufacturing_deliveries;
ALTER TABLE finished_product_units DROP CONSTRAINT finished_product_unit_execution_key;
