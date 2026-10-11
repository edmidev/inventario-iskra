-- +goose Up
-- Stock per location for consumables.
--
-- item_stocks holds one row per (consumable, location) that currently has
-- stock. Policy: a row whose quantity reaches 0 is deleted by the
-- application, so "has stock at this location" means "the row exists".
-- items.stock_current stays as the denormalized total and must always equal
-- SUM(item_stocks.quantity) for consumables; a deferred constraint trigger
-- checks it at commit. Assets never have rows here: each asset has exactly
-- one location in items.location_id.
--
-- For consumables items.location_id is no longer authoritative and is set to
-- NULL by this migration (the location of a consumable lives in item_stocks).
CREATE TABLE item_stocks (
    item_id     UUID        NOT NULL REFERENCES items (id) ON DELETE RESTRICT,
    location_id BIGINT      NOT NULL REFERENCES locations (id) ON DELETE RESTRICT,
    quantity    INTEGER     NOT NULL CHECK (quantity >= 0),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (item_id, location_id)
);

CREATE INDEX item_stocks_location_id_idx ON item_stocks (location_id);

-- Movements gain the location they affected. For in/out/adjust/opening it is
-- location_id; transfers keep from/to and add moved_qty, the quantity moved
-- (qty stays the signed change of the TOTAL, 0 for transfers).
ALTER TABLE stock_movements
    ADD COLUMN location_id BIGINT REFERENCES locations (id) ON DELETE RESTRICT,
    ADD COLUMN moved_qty   INTEGER NOT NULL DEFAULT 0;

-- Backfill. The ledger is append-only, so the immutability trigger is
-- disabled for the duration of this migration only.
-- +goose StatementBegin
DO $$
DECLARE
    v_default BIGINT;
BEGIN
    ALTER TABLE stock_movements DISABLE TRIGGER stock_movements_no_update_delete;

    -- A legacy consumable transfer moved the whole item.
    UPDATE stock_movements m SET moved_qty = m.stock_before
    FROM items i
    WHERE i.id = m.item_id AND i.kind = 'consumable' AND m.type = 'transfer';

    -- Openings happened where the item was created.
    UPDATE stock_movements m SET location_id = m.to_location_id
    FROM items i
    WHERE i.id = m.item_id AND i.kind = 'consumable' AND m.type = 'opening';

    -- in/out/adjust happened wherever the item was at that time: after the
    -- latest earlier opening/transfer, else before the first later transfer,
    -- else at the item's current location.
    UPDATE stock_movements m SET location_id = CASE
        WHEN EXISTS (SELECT 1 FROM stock_movements p
                     WHERE p.item_id = m.item_id AND p.id < m.id AND p.type IN ('opening', 'transfer'))
            THEN (SELECT p.to_location_id FROM stock_movements p
                  WHERE p.item_id = m.item_id AND p.id < m.id AND p.type IN ('opening', 'transfer')
                  ORDER BY p.id DESC LIMIT 1)
        WHEN EXISTS (SELECT 1 FROM stock_movements n
                     WHERE n.item_id = m.item_id AND n.id > m.id AND n.type = 'transfer')
            THEN (SELECT n.from_location_id FROM stock_movements n
                  WHERE n.item_id = m.item_id AND n.id > m.id AND n.type = 'transfer'
                  ORDER BY n.id ASC LIMIT 1)
        ELSE i.location_id
    END
    FROM items i
    WHERE i.id = m.item_id AND i.kind = 'consumable' AND m.type IN ('in', 'out', 'adjust');

    INSERT INTO item_stocks (item_id, location_id, quantity)
    SELECT id, location_id, stock_current FROM items
    WHERE kind = 'consumable' AND stock_current > 0 AND location_id IS NOT NULL;

    -- Whatever still has no location belongs to "Sin ubicación", created only
    -- when something needs it.
    IF EXISTS (SELECT 1 FROM stock_movements m JOIN items i ON i.id = m.item_id
               WHERE i.kind = 'consumable' AND m.location_id IS NULL AND m.type IN ('opening', 'in', 'out', 'adjust'))
       OR EXISTS (SELECT 1 FROM stock_movements m JOIN items i ON i.id = m.item_id
                  WHERE i.kind = 'consumable' AND m.type = 'transfer' AND m.moved_qty > 0 AND m.from_location_id IS NULL)
       OR EXISTS (SELECT 1 FROM items WHERE kind = 'consumable' AND stock_current > 0 AND location_id IS NULL)
    THEN
        SELECT id INTO v_default FROM locations WHERE name = 'Sin ubicación' ORDER BY id LIMIT 1;
        IF v_default IS NULL THEN
            INSERT INTO locations (name, kind) VALUES ('Sin ubicación', 'storage') RETURNING id INTO v_default;
        END IF;
        UPDATE stock_movements m SET location_id = v_default
        FROM items i
        WHERE i.id = m.item_id AND i.kind = 'consumable' AND m.location_id IS NULL AND m.type IN ('opening', 'in', 'out', 'adjust');
        UPDATE stock_movements m SET from_location_id = v_default
        FROM items i
        WHERE i.id = m.item_id AND i.kind = 'consumable' AND m.type = 'transfer' AND m.moved_qty > 0
          AND m.from_location_id IS NULL AND m.to_location_id <> v_default;
        INSERT INTO item_stocks (item_id, location_id, quantity)
        SELECT id, v_default, stock_current FROM items
        WHERE kind = 'consumable' AND stock_current > 0 AND location_id IS NULL;
    END IF;

    UPDATE items SET location_id = NULL WHERE kind = 'consumable' AND location_id IS NOT NULL;

    ALTER TABLE stock_movements ENABLE TRIGGER stock_movements_no_update_delete;
END
$$;
-- +goose StatementEnd

-- A multi-location opening writes one opening movement per location, so only
-- the first has stock_before = 0: relax that part of the original shape check.
ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_shape;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_shape CHECK (
    (type = 'opening'  AND qty > 0)
    OR (type = 'in'    AND qty > 0)
    OR (type = 'out'   AND qty < 0)
    OR (type = 'adjust' AND length(btrim(note)) > 0)
    OR (type = 'transfer' AND qty = 0 AND to_location_id IS NOT NULL
        AND from_location_id IS DISTINCT FROM to_location_id)
);

ALTER TABLE stock_movements
    ADD CONSTRAINT stock_movements_moved_qty_shape CHECK (
        (type = 'transfer' AND moved_qty >= 0) OR (type <> 'transfer' AND moved_qty = 0)),
    ADD CONSTRAINT stock_movements_location_shape CHECK (type <> 'transfer' OR location_id IS NULL);

-- Guard: items.stock_current must equal the sum of the per-location rows for
-- consumables, and assets must have no rows. Deferred so a transaction may
-- change both sides in any order; it is checked once at commit.
-- +goose StatementBegin
CREATE FUNCTION item_stock_total_check() RETURNS trigger AS $$
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

CREATE CONSTRAINT TRIGGER item_stocks_total_check
    AFTER INSERT OR UPDATE OR DELETE ON item_stocks
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION item_stock_total_check();

CREATE CONSTRAINT TRIGGER items_stock_total_check
    AFTER INSERT OR UPDATE OF stock_current, kind ON items
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION item_stock_total_check();

-- +goose Down
DROP TRIGGER items_stock_total_check ON items;
DROP TRIGGER item_stocks_total_check ON item_stocks;
DROP FUNCTION item_stock_total_check();

-- Consumables go back to a single location: the one holding the most stock.
UPDATE items i SET location_id = (
    SELECT s.location_id FROM item_stocks s WHERE s.item_id = i.id
    ORDER BY s.quantity DESC, s.location_id LIMIT 1)
WHERE i.kind = 'consumable';

ALTER TABLE stock_movements DROP CONSTRAINT stock_movements_shape;
ALTER TABLE stock_movements ADD CONSTRAINT stock_movements_shape CHECK (
    (type = 'opening'  AND qty > 0 AND stock_before = 0)
    OR (type = 'in'    AND qty > 0)
    OR (type = 'out'   AND qty < 0)
    OR (type = 'adjust' AND length(btrim(note)) > 0)
    OR (type = 'transfer' AND qty = 0 AND to_location_id IS NOT NULL
        AND from_location_id IS DISTINCT FROM to_location_id)
);
ALTER TABLE stock_movements
    DROP CONSTRAINT stock_movements_location_shape,
    DROP CONSTRAINT stock_movements_moved_qty_shape,
    DROP COLUMN moved_qty,
    DROP COLUMN location_id;
DROP TABLE item_stocks;
