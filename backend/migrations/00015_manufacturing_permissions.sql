-- +goose Up
ALTER TABLE identity_roles DROP CONSTRAINT supported_permissions;
ALTER TABLE identity_roles ADD CONSTRAINT supported_permissions CHECK (permissions <@ ARRAY['inventory:read','operations:write','items:write','catalogs:manage','users:manage','roles:manage','customers:read','customers:write','projects:read','projects:write']::TEXT[]);
UPDATE identity_roles SET permissions = permissions || ARRAY['customers:read','customers:write','projects:read','projects:write']::TEXT[] WHERE role='admin';

-- +goose Down
UPDATE identity_roles SET permissions = ARRAY(SELECT p FROM unnest(permissions) AS p WHERE p NOT IN ('customers:read','customers:write','projects:read','projects:write'));
ALTER TABLE identity_roles DROP CONSTRAINT supported_permissions;
ALTER TABLE identity_roles ADD CONSTRAINT supported_permissions CHECK (permissions <@ ARRAY['inventory:read','operations:write','items:write','catalogs:manage','users:manage','roles:manage']::TEXT[]);
