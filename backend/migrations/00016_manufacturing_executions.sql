-- +goose Up
ALTER TABLE identity_roles DROP CONSTRAINT supported_permissions;
ALTER TABLE identity_roles ADD CONSTRAINT supported_permissions CHECK (permissions <@ ARRAY['inventory:read','operations:write','items:write','catalogs:manage','users:manage','roles:manage','customers:read','customers:write','projects:read','projects:write','executions:read','executions:write']::TEXT[]);
UPDATE identity_roles SET permissions = permissions || ARRAY['executions:read','executions:write']::TEXT[] WHERE role='admin';

CREATE TABLE manufacturing_executions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_template_id BIGINT NOT NULL REFERENCES project_templates(id) ON DELETE RESTRICT,
    project_snapshot JSONB NOT NULL CHECK(jsonb_typeof(project_snapshot)='object'),
    units INTEGER NOT NULL CHECK(units > 0),
    estimated_days INTEGER NOT NULL CHECK(estimated_days BETWEEN 1 AND 36500),
    status TEXT NOT NULL DEFAULT 'registered' CHECK(status IN ('registered','in_progress','paused','completed','cancelled')),
    principal_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX manufacturing_executions_status_created_idx ON manufacturing_executions(status, created_at DESC, id DESC);

CREATE TABLE manufacturing_execution_customers (
    execution_id BIGINT NOT NULL REFERENCES manufacturing_executions(id) ON DELETE CASCADE,
    customer_id BIGINT NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    PRIMARY KEY(execution_id, customer_id)
);

CREATE TABLE manufacturing_execution_workers (
    execution_id BIGINT NOT NULL REFERENCES manufacturing_executions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    PRIMARY KEY(execution_id, user_id)
);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM manufacturing_executions) THEN
        RAISE EXCEPTION 'cannot roll back execution schema while manufacturing execution history exists';
    END IF;
END $$;
-- +goose StatementEnd
DROP TABLE manufacturing_execution_workers;
DROP TABLE manufacturing_execution_customers;
DROP TABLE manufacturing_executions;

UPDATE identity_roles SET permissions = ARRAY(SELECT p FROM unnest(permissions) AS p WHERE p NOT IN ('executions:read','executions:write'));
ALTER TABLE identity_roles DROP CONSTRAINT supported_permissions;
ALTER TABLE identity_roles ADD CONSTRAINT supported_permissions CHECK (permissions <@ ARRAY['inventory:read','operations:write','items:write','catalogs:manage','users:manage','roles:manage','customers:read','customers:write','projects:read','projects:write']::TEXT[]);
