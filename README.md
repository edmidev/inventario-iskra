# Inventario

Mobile-first inventory system for assets and consumables, with roles, basic custody (loan/return) and maintenance tracking.

## Stack

- Backend: Go, chi, PostgreSQL 16, goose (migrations, applied at API startup), pgx/v5. Hexagonal layout with one module per business area.
- Frontend: React, Vite, TypeScript, Tailwind, TanStack Query, React Router, Vitest.
- Contract: `openapi/openapi.yaml`.

## Layout

```
backend/    Go API (cmd/api, internal/{identity,catalog,stock,custody,maintenance}, migrations)
frontend/   React app (atomic design under src/components)
openapi/    API contract
```

## Run

```sh
cp .env.example .env
docker compose up --build   # web on :3000, api on :8080, health at /api/v1/health
```

Local development:

```sh
make -C backend test run    # run needs DATABASE_URL, JWT_SECRET (see Configuration)
make -C backend migrate     # apply migrations without starting the API
cd frontend && npm install && npm run dev
```

## Configuration

Backend environment variables (set in `.env` for Docker Compose):

| Variable | Required | Description |
| --- | --- | --- |
| `DATABASE_URL` | yes | PostgreSQL connection string (set by Compose). |
| `JWT_SECRET` | yes | HS256 signing key for access tokens, at least 32 characters. The API refuses to start otherwise. |
| `COOKIE_SECURE` | no | `true` (default) marks the refresh cookie `Secure`; Compose defaults it to `false` for plain-HTTP local use. Set `true` behind HTTPS. |
| `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD` | no | Creates the first admin at startup, only when the users table is empty. Password needs 10+ characters. |
| `ACCESS_TOKEN_TTL`, `REFRESH_TOKEN_TTL` | no | Go durations; defaults `15m` and `720h`. |

Generate a secret with `openssl rand -base64 48`. Add the variables above to your `.env` (see `.env.example`).

## Identity and permissions

Auth is a short-lived access JWT (sent as `Authorization: Bearer`) plus a rotating opaque refresh token stored hashed in the database and delivered as an httpOnly, SameSite=Lax cookie scoped to `/api/v1/auth`. Reusing a rotated refresh token revokes all of that user's sessions. Failed logins are throttled per client IP and email (in memory, per process).

| Role | Permissions |
| --- | --- |
| `viewer` | read |
| `operator` | read + operational writes (movements, custody, maintenance) |
| `admin` | everything, including users and catalogs |

The last active admin cannot be deactivated or demoted. Endpoints are documented in `openapi/openapi.yaml`. Other modules reuse `httpx.Authenticate` and `httpx.RequirePermission`.

## Test

```sh
make -C backend test lint
# PostgreSQL integration tests are skipped unless TEST_DATABASE_URL points at a disposable database:
TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/inventario_test?sslmode=disable' make -C backend test
cd frontend && npm test -- --run && npm run build
```
