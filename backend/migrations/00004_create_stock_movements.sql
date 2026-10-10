-- +goose Up
-- Append-only ledger of every stock or location change. `qty` is the signed
-- delta actually applied to stock (0 for transfers). user_id is NULL only for
-- rows written by this migration or by system imports.
CREATE TABLE stock_movements (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    item_id          UUID NOT NULL REFERENCES items (id) ON DELETE RESTRICT,
    type             TEXT NOT NULL CHECK (type IN ('opening', 'in', 'out', 'adjust', 'transfer')),
    qty              INTEGER NOT NULL,
    stock_before     INTEGER NOT NULL CHECK (stock_before >= 0),
    stock_after      INTEGER NOT NULL CHECK (stock_after >= 0),
    from_location_id BIGINT REFERENCES locations (id) ON DELETE RESTRICT,
    to_location_id   BIGINT REFERENCES locations (id) ON DELETE RESTRICT,
    user_id          UUID REFERENCES users (id) ON DELETE RESTRICT,
    note             TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT stock_movements_balance CHECK (stock_after = stock_before + qty),
    CONSTRAINT stock_movements_shape CHECK (
        (type = 'opening'  AND qty > 0 AND stock_before = 0)
        OR (type = 'in'    AND qty > 0)
        OR (type = 'out'   AND qty < 0)
        OR (type = 'adjust' AND length(btrim(note)) > 0)
        OR (type = 'transfer' AND qty = 0 AND to_location_id IS NOT NULL
            AND from_location_id IS DISTINCT FROM to_location_id)
    )
);

CREATE INDEX stock_movements_item_history_idx ON stock_movements (item_id, created_at DESC, id DESC);

-- Existing consumables get an opening movement so history reconciles with
-- items.stock_current from day one.
INSERT INTO stock_movements (item_id, type, qty, stock_before, stock_after, to_location_id, note, created_at)
SELECT id, 'opening', stock_current, 0, stock_current, location_id, 'Opening balance (migration)', created_at
FROM items
WHERE kind = 'consumable' AND stock_current > 0;

-- +goose StatementBegin
CREATE FUNCTION stock_movements_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'stock_movements is append-only (% rejected)', TG_OP
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER stock_movements_no_update_delete
    BEFORE UPDATE OR DELETE ON stock_movements
    FOR EACH ROW EXECUTE FUNCTION stock_movements_immutable();

-- +goose Down
DROP TRIGGER stock_movements_no_update_delete ON stock_movements;
DROP FUNCTION stock_movements_immutable();
DROP TABLE stock_movements;
