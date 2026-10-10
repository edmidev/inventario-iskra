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
| `operator` | read + operational writes (movements, custody, maintenance) + create/edit/delete items |
| `admin` | everything, including users, categories and locations |

The last active admin cannot be deactivated or demoted. Endpoints are documented in `openapi/openapi.yaml`. Other modules reuse `httpx.Authenticate` and `httpx.RequirePermission`.

## Catalog

Categories, locations and items live in the `catalog` module (`/api/v1/categories`, `/locations`, `/items`; see `openapi/openapi.yaml`).

- Item kinds: `asset` (unique, stock always 1, no unit) and `consumable` (requires a unit, tracks `stock_current` / `stock_min`). `kind` and `code` never change after creation. `stock_current` is only set as an opening balance on create; later changes belong to stock movements.
- Codes are generated as `ISK-<category prefix>-<sequence>` (3+ digits) from an atomic per-prefix counter. Only admins may supply a code (legacy or imported); it must match `ISK-[A-Z0-9]+(-[A-Z0-9]+)*`. Deleting an item is a soft delete and its code stays reserved. Look items up by code with `GET /items/by-code/{code}` (QR scanning).
- `availability` (`available` / `unavailable`) is derived, never stored: today it means `status = functional`; custody will also require no active loan.
- Categories, locations: admin only to write, any role reads. Items: operator and admin write, viewer reads. Deleting a category with items, or a location with child locations or items, returns 409.
- Search: `GET /items?q=` matches code and description (case-insensitive). Filters: `category_id`, `status`, `kind`, `availability`, `location_id`, `low_stock=true`; `sort` (`code`, `description`, `updated_at`; prefix `-` or `order=desc` for descending), `limit` (max 100, default 25) and `offset`. The response includes `total`. A `pg_trgm` index is created when the extension is available; search works without it.
- Validation errors return 422 `validation_failed` with `error.fields`.

## Stock

Movements and low-stock alerts live in the `stock` module (`POST|GET /api/v1/items/{id}/movements`, `GET /api/v1/stock/alerts`; see `openapi/openapi.yaml`). No new environment variables.

- `stock_movements` is an append-only ledger: a database trigger rejects `UPDATE` and `DELETE`. Each row stores the signed `qty` delta, `stock_before`, `stock_after`, from/to location, the acting user and a note. Corrections are new movements, never edits.
- Types: `in` (adds), `out` (subtracts, never below 0, otherwise 409 `insufficient_stock`), `adjust` (sets an absolute counted quantity; requires a note; the movement records the delta), `transfer` (moves an asset or consumable to another location; stock unchanged) and `opening` (system-written).
- A `transfer` of an asset that is on loan is rejected with 409 `item_on_loan` (checked inside the same row-locked transaction against active `custody_assignments`; the stock domain only receives an `on_loan` flag).
- `in`, `out` and `adjust` apply to consumables only; assets get 422 `not_applicable` but can be transferred. Soft-deleted items return 404.
- Each movement locks the item row (`SELECT ... FOR UPDATE`) and updates `items.stock_current` / `location_id` and inserts the movement in one transaction, so parallel `out` requests never oversell and the ledger always reconciles with the item.
- Creating a consumable with `stock_current > 0` records an `opening` movement in the same transaction. Catalog exposes an adapter-level create hook (`catalogpg.WithCreateHook`) that `cmd/api/wire.go` fills with stock's `RecordOpening`; catalog never imports stock. Migration `00004` backfills an `opening` movement for consumables that already had stock.
- Operators and admins record movements (`items:write`); every role reads history (`limit` max 100, default 25, `offset`, optional `type`) and alerts (consumables with `stock_min > 0` and `stock_current <= stock_min`, most severe first).

## Custody

Loans and returns live in the `custody` module (`POST /api/v1/items/{id}/assign`, `POST /items/{id}/return`, `GET /items/{id}/custody`, `GET /custody/active`; see `openapi/openapi.yaml`). No new environment variables; migration `00005` creates `custody_assignments`.

- Only `asset` items can be lent (consumables: 422 `not_applicable`, they move with stock transfers). The assignee is an existing location of kind `person` or `container` (for example "Maleta de Alejandro"). The item must be `functional` (409 `item_not_available`) and not already on loan (409 `already_assigned`); `due_at` is optional and must be in the future.
- Assigning moves the item's `location_id` to the assignee and remembers the previous location as `origin_location_id`. Returning closes the assignment and moves the item to `return_location_id` or, by default, back to the origin (left without location if the origin was deleted). Returning without an active loan is 409 `not_assigned`; an item that broke while on loan can still be returned (its status is untouched).
- Each operation locks the item row (`SELECT ... FOR UPDATE`) and writes the assignment and the item in one transaction. A partial unique index (`custody_assignments_one_active_per_item`, `WHERE returned_at IS NULL`) guarantees at most one active loan per item, so parallel assigns yield exactly one success. Rows are never deleted: history keeps the assignee, origin, return location, both users and both notes. Locations referenced by history cannot be deleted (409 `location_in_use`).
- Availability is derived, never stored: `available` means `functional` and no active loan. Item responses add `on_loan` and, while on loan, `custody` (`assignee_location_id`, `assignee_name`, `assigned_at`, `due_at`, `overdue`); `GET /items?availability=` honors loans. The catalog's postgres adapter reads `custody_assignments` with a LEFT JOIN for this; the catalog domain and app layers never import custody.
- While an item is on loan custody owns its location: deleting it or changing `location_id` through `PATCH /items/{id}` returns 409 `item_on_loan`.
- Operators and admins assign and return (`items:write`); every role reads an item's history and `GET /custody/active` (overdue first, then oldest; filters `assignee_location_id`, `overdue=true`, `q`; `limit` max 100, default 25, `offset`).

## Test

```sh
make -C backend test lint
# PostgreSQL integration tests are skipped unless TEST_DATABASE_URL points at a disposable database:
TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/inventario_test?sslmode=disable' make -C backend test
cd frontend && npm test -- --run && npm run build
```
