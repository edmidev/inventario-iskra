-- +goose Up
CREATE TABLE manufacturing_execution_purchase_links (
    shortage_request_id BIGINT NOT NULL REFERENCES manufacturing_execution_shortage_requests(id) ON DELETE RESTRICT,
    purchase_order_line_id BIGINT NOT NULL UNIQUE REFERENCES purchase_order_lines(id) ON DELETE RESTRICT,
    quantity NUMERIC(18,3) NOT NULL CHECK (quantity > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (shortage_request_id, purchase_order_line_id)
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_execution_purchase_links) THEN
        RAISE EXCEPTION 'cannot roll back execution purchase links after link history exists';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TABLE manufacturing_execution_purchase_links;
