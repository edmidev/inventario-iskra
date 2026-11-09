-- +goose Up
CREATE TABLE manufacturing_execution_supplemental_tools (
    execution_id BIGINT NOT NULL REFERENCES manufacturing_executions(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (execution_id, item_id)
);

CREATE TABLE manufacturing_execution_tool_additions (
    request_id UUID PRIMARY KEY,
    execution_id BIGINT NOT NULL,
    item_id UUID NOT NULL,
    actor_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (execution_id, item_id)
        REFERENCES manufacturing_execution_supplemental_tools(execution_id, item_id) ON DELETE RESTRICT
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_execution_tool_additions)
        OR EXISTS (SELECT 1 FROM manufacturing_execution_supplemental_tools) THEN
        RAISE EXCEPTION 'cannot roll back supplemental tools while execution history exists';
    END IF;
END;
$$;
-- +goose StatementEnd

DROP TABLE manufacturing_execution_tool_additions;
DROP TABLE manufacturing_execution_supplemental_tools;
