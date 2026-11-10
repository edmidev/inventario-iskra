-- +goose Up
-- Linked receipts keep their reservation priority through later stock
-- movements and counts. All other available stock remains FIFO.
-- Callers hold the item's row lock before invoking this function.
ALTER TABLE manufacturing_execution_materials
ADD COLUMN quantity_earmarked_consumed NUMERIC(18,3) NOT NULL DEFAULT 0
CHECK (quantity_earmarked_consumed >= 0 AND quantity_earmarked_consumed <= quantity_delivered);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION rebalance_manufacturing_execution_materials(p_item_id UUID)
RETURNS VOID AS $$
BEGIN
    WITH active_needs AS (
        SELECT m.execution_id, e.created_at, i.stock_current,
               GREATEST(m.quantity_required-m.quantity_delivered,0) AS needed,
               GREATEST(0,COALESCE((
                   SELECT SUM(LEAST(link.quantity,l.received_qty))
                   FROM manufacturing_execution_purchase_links link
                   JOIN manufacturing_execution_shortage_requests r ON r.id=link.shortage_request_id
                   JOIN purchase_order_lines l ON l.id=link.purchase_order_line_id
                   WHERE r.execution_id=m.execution_id AND r.status='open'
                     AND l.item_id=m.item_id
               ),0)-m.quantity_earmarked_consumed) AS earmarked
        FROM manufacturing_execution_materials m
        JOIN manufacturing_executions e ON e.id=m.execution_id
        JOIN items i ON i.id=m.item_id
        WHERE m.item_id=p_item_id AND e.status IN ('registered','in_progress','paused')
    ), prioritized AS (
        SELECT *, LEAST(needed,earmarked) AS priority_need,
               SUM(LEAST(needed,earmarked)) OVER (
                   ORDER BY created_at,execution_id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
               ) AS cumulative_priority
        FROM active_needs
    ), first_pass AS (
        SELECT *, GREATEST(0,LEAST(priority_need,stock_current-(cumulative_priority-priority_need))) AS priority_reserved
        FROM prioritized
    ), second_pass AS (
        SELECT *, needed-priority_reserved AS residual_need,
               stock_current-SUM(priority_reserved) OVER () AS residual_stock,
               SUM(needed-priority_reserved) OVER (
                   ORDER BY created_at,execution_id ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
               ) AS cumulative_residual
        FROM first_pass
    ), allocated AS (
        SELECT execution_id,
               priority_reserved+
               GREATEST(0,LEAST(residual_need,residual_stock-(cumulative_residual-residual_need))) AS reserved
        FROM second_pass
    )
    UPDATE manufacturing_execution_materials m
    SET quantity_reserved=a.reserved
    FROM allocated a
    WHERE m.item_id=p_item_id AND m.execution_id=a.execution_id;

    UPDATE manufacturing_execution_materials m SET quantity_reserved=0
    WHERE m.item_id=p_item_id AND NOT EXISTS (
        SELECT 1 FROM manufacturing_executions e
        WHERE e.id=m.execution_id AND e.status IN ('registered','in_progress','paused'));
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_execution_purchase_links) THEN
        RAISE EXCEPTION 'cannot roll back earmarked receipts after purchase link history exists';
    END IF;
END;
$$;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION rebalance_manufacturing_execution_materials(p_item_id UUID)
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
ALTER TABLE manufacturing_execution_materials DROP COLUMN quantity_earmarked_consumed;
