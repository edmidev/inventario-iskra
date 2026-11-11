-- +goose Up
DROP TRIGGER items_stock_total_check ON items;
-- Preserve human-unit values; applications use exact integer thousandths in memory.
ALTER TABLE items
    ALTER COLUMN stock_current TYPE NUMERIC(16,3) USING stock_current::numeric,
    ALTER COLUMN stock_min TYPE NUMERIC(16,3) USING stock_min::numeric;
ALTER TABLE item_stocks
    ALTER COLUMN quantity TYPE NUMERIC(16,3) USING quantity::numeric;
ALTER TABLE stock_movements
    ALTER COLUMN qty TYPE NUMERIC(16,3) USING qty::numeric,
    ALTER COLUMN stock_before TYPE NUMERIC(16,3) USING stock_before::numeric,
    ALTER COLUMN stock_after TYPE NUMERIC(16,3) USING stock_after::numeric,
    ALTER COLUMN moved_qty TYPE NUMERIC(16,3) USING moved_qty::numeric;
ALTER TABLE purchase_order_lines
    ALTER COLUMN quantity TYPE NUMERIC(16,3) USING quantity::numeric,
    ALTER COLUMN received_qty TYPE NUMERIC(16,3) USING received_qty::numeric;
ALTER TABLE po_receipt_lines
    ALTER COLUMN quantity TYPE NUMERIC(16,3) USING quantity::numeric;
ALTER TABLE po_receipt_items
    ALTER COLUMN quantity TYPE NUMERIC(16,3) USING quantity::numeric;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION item_stock_total_check() RETURNS trigger AS $$
DECLARE
    v_item  UUID;
    v_kind  TEXT;
    v_total NUMERIC;
    v_sum   NUMERIC;
BEGIN
    IF TG_TABLE_NAME = 'items' THEN
        v_item := NEW.id;
    ELSIF TG_OP = 'DELETE' THEN
        v_item := OLD.item_id;
    ELSE
        v_item := NEW.item_id;
    END IF;
    SELECT kind, stock_current INTO v_kind, v_total FROM items WHERE id = v_item;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    SELECT COALESCE(SUM(quantity), 0) INTO v_sum FROM item_stocks WHERE item_id = v_item;
    IF v_kind = 'consumable' AND v_sum <> v_total THEN
        RAISE EXCEPTION 'items.stock_current (%) must equal the sum of item_stocks (%) for item %', v_total, v_sum, v_item
            USING ERRCODE = 'check_violation', CONSTRAINT = 'item_stock_total_matches';
    END IF;
    IF v_kind = 'asset' AND v_sum <> 0 THEN
        RAISE EXCEPTION 'assets have no per-location stock (item %)', v_item
            USING ERRCODE = 'check_violation', CONSTRAINT = 'item_stock_total_matches';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER items_stock_total_check
    AFTER INSERT OR UPDATE OF stock_current, kind ON items
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION item_stock_total_check();

-- +goose Down
-- Never silently round fractional historical values during rollback.
-- +goose StatementBegin
DO $$
BEGIN
    LOCK TABLE items IN ACCESS EXCLUSIVE MODE;
    LOCK TABLE item_stocks IN ACCESS EXCLUSIVE MODE;
    LOCK TABLE stock_movements IN ACCESS EXCLUSIVE MODE;
    LOCK TABLE purchase_order_lines IN ACCESS EXCLUSIVE MODE;
    LOCK TABLE po_receipt_lines IN ACCESS EXCLUSIVE MODE;
    LOCK TABLE po_receipt_items IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM items WHERE stock_current <> trunc(stock_current) OR stock_min <> trunc(stock_min)) THEN
        RAISE EXCEPTION 'cannot roll back decimal quantities: fractional values exist in items';
    END IF;
    IF EXISTS (SELECT 1 FROM item_stocks WHERE quantity <> trunc(quantity)) THEN
        RAISE EXCEPTION 'cannot roll back decimal quantities: fractional values exist in item_stocks';
    END IF;
    IF EXISTS (SELECT 1 FROM stock_movements WHERE qty <> trunc(qty) OR stock_before <> trunc(stock_before) OR stock_after <> trunc(stock_after) OR moved_qty <> trunc(moved_qty)) THEN
        RAISE EXCEPTION 'cannot roll back decimal quantities: fractional values exist in stock_movements';
    END IF;
    IF EXISTS (SELECT 1 FROM purchase_order_lines WHERE quantity <> trunc(quantity) OR received_qty <> trunc(received_qty)) THEN
        RAISE EXCEPTION 'cannot roll back decimal quantities: fractional values exist in purchase_order_lines';
    END IF;
    IF EXISTS (SELECT 1 FROM po_receipt_lines WHERE quantity <> trunc(quantity)) THEN
        RAISE EXCEPTION 'cannot roll back decimal quantities: fractional values exist in po_receipt_lines';
    END IF;
    IF EXISTS (SELECT 1 FROM po_receipt_items WHERE quantity <> trunc(quantity)) THEN
        RAISE EXCEPTION 'cannot roll back decimal quantities: fractional values exist in po_receipt_items';
    END IF;
    IF EXISTS (SELECT 1 FROM purchase_order_lines WHERE new_item IS NOT NULL AND (new_item->>'stock_min')::numeric <> trunc((new_item->>'stock_min')::numeric)) THEN
        RAISE EXCEPTION 'cannot roll back decimal quantities: fractional new item minimum exists';
    END IF;
END;
$$;
-- +goose StatementEnd
DROP TRIGGER items_stock_total_check ON items;
ALTER TABLE items
    ALTER COLUMN stock_current TYPE INTEGER USING stock_current::integer,
    ALTER COLUMN stock_min TYPE INTEGER USING stock_min::integer;
ALTER TABLE item_stocks
    ALTER COLUMN quantity TYPE INTEGER USING quantity::integer;
ALTER TABLE stock_movements
    ALTER COLUMN qty TYPE INTEGER USING qty::integer,
    ALTER COLUMN stock_before TYPE INTEGER USING stock_before::integer,
    ALTER COLUMN stock_after TYPE INTEGER USING stock_after::integer,
    ALTER COLUMN moved_qty TYPE INTEGER USING moved_qty::integer;
ALTER TABLE purchase_order_lines
    ALTER COLUMN quantity TYPE INTEGER USING quantity::integer,
    ALTER COLUMN received_qty TYPE INTEGER USING received_qty::integer;
ALTER TABLE po_receipt_lines
    ALTER COLUMN quantity TYPE INTEGER USING quantity::integer;
ALTER TABLE po_receipt_items
    ALTER COLUMN quantity TYPE INTEGER USING quantity::integer;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION item_stock_total_check() RETURNS trigger AS $$
DECLARE
    v_item  UUID;
    v_kind  TEXT;
    v_total INTEGER;
    v_sum   BIGINT;
BEGIN
    IF TG_TABLE_NAME = 'items' THEN
        v_item := NEW.id;
    ELSIF TG_OP = 'DELETE' THEN
        v_item := OLD.item_id;
    ELSE
        v_item := NEW.item_id;
    END IF;
    SELECT kind, stock_current INTO v_kind, v_total FROM items WHERE id = v_item;
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    SELECT COALESCE(SUM(quantity), 0) INTO v_sum FROM item_stocks WHERE item_id = v_item;
    IF v_kind = 'consumable' AND v_sum <> v_total THEN
        RAISE EXCEPTION 'items.stock_current (%) must equal the sum of item_stocks (%) for item %', v_total, v_sum, v_item
            USING ERRCODE = 'check_violation', CONSTRAINT = 'item_stock_total_matches';
    END IF;
    IF v_kind = 'asset' AND v_sum <> 0 THEN
        RAISE EXCEPTION 'assets have no per-location stock (item %)', v_item
            USING ERRCODE = 'check_violation', CONSTRAINT = 'item_stock_total_matches';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER items_stock_total_check
    AFTER INSERT OR UPDATE OF stock_current, kind ON items
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION item_stock_total_check();
