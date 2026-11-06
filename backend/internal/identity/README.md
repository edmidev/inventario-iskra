# Dynamic authorization

User role identities remain fixed (`admin`, `operator`, `viewer`, `workshop_chief`,
`supervisor`, `inventory_chief`, `worker`), while their grants are stored in
`identity_roles.permissions`. Migration 00012 preserves legacy grants and seeds
new roles without grants. `admin` always retains `users:manage` and `roles:manage`;
the existing last-active-admin invariant still applies to account changes.

Authenticated API requests verify the bearer signature, then load the current
active user and current role grants. JWT role claims are not authorization
sources. Changes apply on the next request, without waiting for token expiry.
Permission lookup failure fails closed. Login/refresh `user` and `GET /auth/me`
include a `permissions` array, including an empty array when no grants exist.

All configuration routes require `roles:manage`:

- `GET /roles`: `{ "data": [{ "role": "operator", "permissions": [] }] }`.
- `GET /permissions`: `{ "data": ["inventory:read", "operations:write", ...] }`.
- `PUT /roles/{role}/permissions`: replaces the entire grant set using
  `{ "permissions": ["inventory:read"] }`, returning the role configuration.
  Unknown roles/permissions, duplicate grants, null/missing arrays, and removing
  administrator recovery grants return HTTP 400. An empty array clears a
  non-admin role's grants. Requests without the management grant return 403.

The capability catalog lives in the domain; adding business capabilities requires
an application change and a corresponding migration to expand the database
constraint. Assigning those capabilities to roles does not require code changes.
Repositories also validate replacements and PostgreSQL independently prevents
removing administrator recovery grants.

Rollback to the original user-role constraint refuses while users hold a new
role, rather than silently reassigning or deleting accounts. Deploy API and
frontend permission-aware changes together; old binaries still use static grants
and must not serve authenticated traffic after grant customization is enabled.
