-- +goose Up
-- Purchase orders, their lines and the receipts that bring goods in.
--
-- Money is stored as integer minor units (cents / céntimos) in the order's
-- currency; exchange_rate (VES per USD) is reference data only and is never
-- used to convert stored amounts.
CREATE TABLE purchase_orders (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    supplier_id   BIGINT NOT NULL REFERENCES suppliers (id) ON DELETE RESTRICT,
    status        TEXT NOT NULL DEFAULT 'draft'
                  CHECK (status IN ('draft', 'ordered', 'partially_received', 'received', 'cancelled')),
    currency      TEXT NOT NULL CHECK (currency IN ('USD', 'VES')),
    exchange_rate NUMERIC(18, 6) CHECK (exchange_rate IS NULL OR exchange_rate > 0),
    expected_at   DATE,
    notes         TEXT NOT NULL DEFAULT '',
    created_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    ordered_at    TIMESTAMPTZ,
    cancelled_at  TIMESTAMPTZ
);

CREATE INDEX purchase_orders_supplier_idx ON purchase_orders (supplier_id);
CREATE INDEX purchase_orders_status_idx ON purchase_orders (status);

-- A line orders an existing item (item_id) or carries the definition of an
-- item to create on first receipt (new_item). Once a new_item line is
-- received, item_id holds the first item created for it and new_item stays as
-- history; the full list of created items lives in po_receipt_items.
CREATE TABLE purchase_order_lines (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id        BIGINT NOT NULL REFERENCES purchase_orders (id) ON DELETE CASCADE,
    position        INTEGER NOT NULL,
    item_id         UUID REFERENCES items (id) ON DELETE RESTRICT,
    new_item        JSONB CHECK (new_item IS NULL OR jsonb_typeof(new_item) = 'object'),
    description     TEXT NOT NULL DEFAULT '',
    quantity        INTEGER NOT NULL CHECK (quantity > 0),
    received_qty    INTEGER NOT NULL DEFAULT 0,
    unit_cost_minor BIGINT NOT NULL CHECK (unit_cost_minor >= 0),
    CONSTRAINT purchase_order_lines_received_range CHECK (received_qty >= 0 AND received_qty <= quantity),
    CONSTRAINT purchase_order_lines_item_or_new CHECK (item_id IS NOT NULL OR new_item IS NOT NULL)
);

CREATE INDEX purchase_order_lines_order_idx ON purchase_order_lines (order_id, position);

CREATE TABLE po_receipts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    BIGINT NOT NULL REFERENCES purchase_orders (id) ON DELETE CASCADE,
    location_id BIGINT NOT NULL REFERENCES locations (id) ON DELETE RESTRICT,
    received_by UUID REFERENCES users (id) ON DELETE SET NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    note        TEXT NOT NULL DEFAULT '',
    client_key  TEXT,
    CONSTRAINT po_receipts_order_client_key_uq UNIQUE (order_id, client_key)
);

CREATE INDEX po_receipts_order_idx ON po_receipts (order_id, id);

CREATE TABLE po_receipt_lines (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    receipt_id BIGINT NOT NULL REFERENCES po_receipts (id) ON DELETE CASCADE,
    line_id    BIGINT NOT NULL REFERENCES purchase_order_lines (id) ON DELETE CASCADE,
    quantity   INTEGER NOT NULL CHECK (quantity > 0)
);

CREATE INDEX po_receipt_lines_receipt_idx ON po_receipt_lines (receipt_id);

-- Items brought in by a receipt line: one row per created asset (quantity 1),
-- or one row with the received quantity for a consumable.
CREATE TABLE po_receipt_items (
    receipt_line_id BIGINT NOT NULL REFERENCES po_receipt_lines (id) ON DELETE CASCADE,
    item_id         UUID NOT NULL REFERENCES items (id) ON DELETE RESTRICT,
    quantity        INTEGER NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (receipt_line_id, item_id)
);

-- +goose Down
DROP TABLE po_receipt_items;
DROP TABLE po_receipt_lines;
DROP TABLE po_receipts;
DROP TABLE purchase_order_lines;
DROP TABLE purchase_orders;
