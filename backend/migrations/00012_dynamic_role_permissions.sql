-- +goose Up
CREATE TABLE identity_roles (
 role TEXT PRIMARY KEY,
 permissions TEXT[] NOT NULL DEFAULT '{}',
 CONSTRAINT admin_permissions CHECK (role <> 'admin' OR permissions @> ARRAY['roles:manage','users:manage']::TEXT[]),
 CONSTRAINT supported_permissions CHECK (permissions <@ ARRAY['inventory:read','operations:write','items:write','catalogs:manage','users:manage','roles:manage']::TEXT[])
);
INSERT INTO identity_roles(role,permissions) VALUES
 ('admin',ARRAY['inventory:read','operations:write','items:write','catalogs:manage','users:manage','roles:manage']),
 ('operator',ARRAY['inventory:read','operations:write','items:write']),
 ('viewer',ARRAY['inventory:read']),
 ('workshop_chief','{}'),('supervisor','{}'),('inventory_chief','{}'),('worker','{}');
ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_fkey FOREIGN KEY(role) REFERENCES identity_roles(role);

-- +goose Down
-- Refuse rollback while new roles are assigned instead of dropping or changing users.
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK(role IN ('admin','operator','viewer'));
ALTER TABLE users DROP CONSTRAINT users_role_fkey;
DROP TABLE identity_roles;
