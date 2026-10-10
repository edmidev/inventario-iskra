# Inventario

Mobile-first inventory system for assets and consumables, with roles, basic custody (loan/return) and maintenance tracking.

## Stack

- Backend: Go, chi, PostgreSQL 16, goose (migrations, applied at API startup), pgx/v5. Hexagonal layout with one module per business area.
- Frontend: React 19, Vite, TypeScript, Tailwind v4, Radix primitives, TanStack Query, React Router, react-hook-form + zod, Vitest + Testing Library + MSW, oxlint.
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
- `availability` (`available` / `unavailable`) is derived, never stored: `available` means `status = functional` and no active loan (custody), `unavailable` otherwise.
- While an item has an open failure (maintenance), `PATCH /items/{id}` cannot change its `status` (409 `item_under_repair`; re-sending the current status is fine). Report and close failures through maintenance. The check runs under the same item row lock; catalog reads `maintenance_records` with adapter-level SQL only.
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

## Maintenance

Preventive maintenance and the failure/repair flow live in the `maintenance` module (`POST|GET /api/v1/items/{id}/maintenance`, `PATCH /maintenance/{id}`, `POST /maintenance/{id}/close`, `GET /maintenance`, `GET /maintenance/due`; see `openapi/openapi.yaml`). No new environment variables; migration `00006` creates `maintenance_records`. It replaces the "Inactivos y Reparación" sheet: failure date, description and destination (for example "Taller de Reparación Interno" or "Espera de Repuestos") are fields of a failure record.

- Only `asset` items have maintenance (consumables: 422 `not_applicable`); soft-deleted items return 404. Records are never deleted.
- **Report failure** (`kind: corrective`): creates an *open* record (`reported_at` defaults to now and cannot be in the future) and sets the item to `inoperative` (default) or `in_repair` (`item_status`). A second open failure for the same item is 409 `open_failure_exists`; a partial unique index (`maintenance_records_one_open_failure_per_item`) guarantees at most one, so parallel reports yield exactly one 201. Reporting works while the item is on loan: custody is not touched and the item can still be returned.
- **Edit** (`PATCH`): description, destination (`null` clears it) and moving the item between `inoperative` and `in_repair`. **Close** (`POST .../close`): optional `closed_at`, `cost`, `resolution_note` and `item_status_after` (`functional` by default, or `inoperative`); a functional item gets `last_maintenance_at = closed_at`. Closed records are immutable (409 `record_closed`).
- **Register preventive** (`kind: preventive`): created closed with `performed_at` (default now, not in the future), optional `cost` and `next_due_at` (derived as `performed_at` + `maintenance_interval_days` when omitted); sets `last_maintenance_at` (never moving it backwards) and does not change the item's status.
- Every write locks the item row (`SELECT ... FOR UPDATE`) and updates the record and the `items` row (`status` / `last_maintenance_at`) in one transaction; maintenance writes the `items` columns with adapter-level SQL and never imports the catalog domain or app.
- Due maintenance is derived, never stored: an asset with `maintenance_interval_days` is due at `COALESCE(last_maintenance_at, created_at) + interval` (24-hour days) and overdue once that is in the past. Item responses add `maintenance_due_at` (null without an interval) and `maintenance_overdue`. `GET /maintenance/due` lists overdue assets plus those due within `within_days` (0 to 365, default 0 = overdue only), most overdue first; filters `category_id`, `q`; `limit` max 100.
- `GET /maintenance` lists records across items, open first then newest, with item code/description and user names; filters `status`, `kind`, `destination` (exact, case-insensitive), `item_id`, `q` (item code/description and record description).
- Operators and admins write (`items:write`); every role reads. Validation errors return 422 `validation_failed` with `error.fields`.

## Dashboard

The home-screen summary lives in the `dashboard` module, a read-only projection (`GET /api/v1/dashboard/summary`; see `openapi/openapi.yaml`). No new environment variables and no migration. Any authenticated role may read it.

- One response with item counts (`items`: total, assets, consumables), asset counts (`assets`: `available`, `unavailable`, `on_loan`, `inoperative`, `in_repair`; available and unavailable partition the assets, the rest overlap them), `low_stock` (`count` plus up to 5 most severe, by lowest `stock_current / stock_min`), `loans` (`active`, `overdue`, up to 5 oldest overdue) and `maintenance` (`open_failures`, `overdue`, up to 5 most overdue). Soft-deleted items are excluded everywhere.
- It reuses the rules of the other modules: low stock is a consumable with `stock_min > 0` and `stock_current <= stock_min`; available is `functional` with no active loan; a loan is overdue when `due_at` is before now; maintenance is overdue when `COALESCE(last_maintenance_at, created_at) + interval` is before now. Time comes from an injectable app clock.
- It costs five SQL queries (one counters query, one open-failures count, three bounded top lists), no N+1. The postgres adapter reads tables owned by catalog, custody and maintenance with adapter-level SQL; the module never imports their domain or app packages.

## Frontend

Mobile-first React app (one-handed use, 44px touch targets, sticky bottom nav and safe-area insets on phones; sidebar and table-like list from `md` up). UI copy is Spanish; code, comments and tests are English. No external fonts or CDN requests.

Scripts (`cd frontend`):

| Script | What it does |
| --- | --- |
| `npm run dev` | Vite dev server on :5173. `/api` is proxied to `http://localhost:8080` (override with `API_PROXY_TARGET`), so no CORS or app env is needed. |
| `npm run lint` | oxlint (typescript, react, jsx-a11y, vitest rules). typescript-eslint does not support TypeScript 7 yet, hence oxlint. |
| `npm run typecheck` | `tsc --noEmit`. |
| `npm test -- --run` | Vitest. The API is mocked with MSW at the network level, so tests exercise the real client, hooks and pages. |
| `npm run build` | `tsc -b && vite build` into `dist/`. In production nginx serves `dist/` and proxies `/api/` to the `api` service (`frontend/nginx.conf`); the app always calls the same-origin `/api/v1`. |

Architecture (`frontend/src`):

```
lib/api/          fetch client, zod schemas, ApiError mapping, in-memory token store
features/<name>/  api/ (TanStack Query hooks), feature logic (filters, form mapping), containers
components/       atoms -> molecules -> organisms; presentational, prop-driven, no data fetching
pages/            route-level containers that fetch and compose organisms
test/             MSW server, fixtures, render helpers
```

Behavior worth knowing:

- Session: the access token lives in memory only (never localStorage). On load the app calls `POST /auth/refresh` (httpOnly cookie) to restore the session. A 401 triggers one shared silent refresh and a single retry; if that fails the user is sent to the login page.
- Errors: the API envelope `{error:{code,message,fields?}}` becomes an `ApiError`; 422 `fields` are mapped onto the matching form inputs, other codes are shown as readable Spanish messages.
- Roles: `viewer` is read-only (no create, edit or delete controls); `operator` and `admin` write items; `admin` additionally sees the custom item code field. Catalog and user management screens arrive with the admin task.
- Items list: search, filters and sort live in the URL (`?q=&status=&category_id=&...`), so refresh, back and shared links keep the view; pagination is "Cargar más" with limit/offset.
- Theming: semantic tokens in `src/index.css` (`@theme`) switch with `prefers-color-scheme` (light and dark).
- Not built yet: stock movements, loans, maintenance flows and the history tabs (T6c), QR scanning, admin screens and PWA (T6d).

## Test

```sh
make -C backend test lint
# PostgreSQL integration tests are skipped unless TEST_DATABASE_URL points at a disposable database:
TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/inventario_test?sslmode=disable' make -C backend test
cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build
```
