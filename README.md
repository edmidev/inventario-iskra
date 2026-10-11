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
| `npm run typecheck` | `tsc --noEmit` for the app and for `e2e/`. |
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
- Roles: `viewer` is read-only (no create, edit, delete or operation controls); `operator` and `admin` write items and run operations; `admin` additionally sees the custom item code field and the administration screens (see below).
- Items list: search, filters and sort live in the URL (`?q=&status=&category_id=&...`), so refresh, back and shared links keep the view; pagination is "Cargar más" with limit/offset.
- Item detail operations (operator and admin, hidden for viewers): an action bar opens a bottom sheet per action and every action is at most three taps (open sheet, quick quantity or pick, confirm).
  - Consumables: Entrada, Salida, Ajuste (absolute count, note required) with a large +/- stepper and quick quantities, and Trasladar. The sheet shows the resulting stock and warns when it ends at or below the minimum.
  - Assets: Prestar (people and containers only, optional due date from today), Devolver (optional return location), Trasladar (disabled with a reason while on loan), Reportar falla, Actualizar reparación and Cerrar falla (shown instead of a new report while a failure is open), Registrar mantenimiento (preventive).
  - API errors (`insufficient_stock`, `item_on_loan`, `already_assigned`, `not_assigned`, `item_not_available`, `open_failure_exists`, `record_closed`, `not_applicable`, 422 `fields`) become Spanish messages inside the sheet; every mutation invalidates the item, lists, dashboard, alerts, custody and maintenance queries and shows a toast.
- Item detail tabs: Historial merges stock movements and loan/return events newest first (two paginated streams merged on the client; events older than what an unfinished stream has loaded are held back so "Cargar más" never inserts above what is shown); Mantenimiento lists failures and preventive services with a due or overdue banner (assets only).
- Operational pages (linked from the dashboard cards and "Más"): `/stock-bajo` (alerts by severity with quick Entrada), `/prestamos` (overdue first; filters in the URL: `overdue`, `assignee`, `q`; quick Devolver), `/mantenimiento` (tabs "Fallas abiertas" with quick Cerrar falla and "Por vencer" with a `within_days` window).
- Code splitting: every page is a lazy route chunk (`React.lazy` in `App.tsx`), so the entry chunk no longer carries the pages and forms (about 596 kB before, about 462 kB now; the rest is React, router, TanStack Query and Radix shared by all routes).
- Theming: semantic tokens in `src/index.css` (`@theme`) switch with `prefers-color-scheme` (light and dark).
- Session offline: if `POST /auth/refresh` cannot reach the server the user is not logged out; the app shows "No se pudo conectar para verificar tu sesión" and retries on its own when the network returns. Queries use `networkMode: "always"` so offline failures surface as normal error states instead of endless spinners.

### QR scan and labels

- `/escanear` reads QR codes with the camera (native `BarcodeDetector` when present, otherwise `qr-scanner`, loaded lazily so it stays out of the entry chunk), with torch toggle, clear states for denied permission / no camera / non-HTTPS, and a manual code field that always works. A scanned value can be a bare code (`ISK-HER-001`) or a URL containing `/materiales/codigo/<code>` or `/materiales/by-code/<code>`; it is resolved through `GET /items/by-code/{code}`.
- Labels encode `${origin}/materiales/codigo/<code>`, so a normal phone camera opens the app, which resolves the code and redirects to the item (after login if needed). Camera access needs HTTPS or `localhost`.
- Item detail has an "Etiqueta QR" sheet (QR, code, description, Imprimir, Compartir). `/materiales/etiquetas` (operator and admin) picks several items and prints a sheet of 50 x 30 mm labels; `@media print` hides everything except the labels.

### Administration (admin only)

Reachable from "Más" and guarded by role (other roles see "Sin acceso" and no entries): `/admin/usuarios` (search, role/state filters, create, edit name/role/active, reset password; the last-admin rule is shown as a readable error), `/admin/categorias` (create/edit/delete; prefix is 2-8 uppercase letters or digits; `category_in_use` explained) and `/admin/ubicaciones` (tree by parent, kind storage/container/person, `location_in_use` and `location_cycle` explained). `/cuenta` ("Mi cuenta", every role) changes your own password and signs you out, since the API revokes all refresh tokens.

### PWA

The app is installable ("Inventario Iskra", standalone, light/dark `theme-color`, maskable icon; icons are generated from `public/icons/*.svg` with `node scripts/generate-icons.mjs`). `vite-plugin-pwa` (Workbox `generateSW`) precaches the app shell and static assets only; there is deliberately no runtime caching, `/api` is denied for navigation fallback, and API and auth responses are never cached. Offline, the shell loads and a "Sin conexión" banner is shown; data screens show their error states. Install it from the browser menu ("Instalar app" / "Añadir a pantalla de inicio") on HTTPS. The service worker only exists in production builds (`npm run build && npm run preview`), never in dev or tests. `frontend/nginx.conf` serves `index.html`, `sw.js` and the manifest with `no-cache`, hashed `/assets/*` as immutable and `/api/` as `no-store`.

### End-to-end tests (Playwright, mobile 390x844)

`cd frontend && npx playwright install chromium && npm run e2e`. The global setup starts a throwaway `postgres:16` container (`inv-test-pg`, port 55432), builds and runs the Go API on port 18080, seeds it through the API (needs docker, Go and Node; port 8080 is not used), and Playwright serves the production build with `vite preview` on :14173 proxying `/api`. Everything is torn down afterwards. Specs cover login/search/detail, loan and return, stock salida/entrada with the low-stock list, failure report and close, viewer restrictions, QR manual resolve and printed-label URLs, user/category/location administration and the PWA (manifest, service worker, no API in caches, offline banner). For a visual pass set `E2E_SCREENSHOTS=<dir>` to also save light/dark screenshots plus a layout audit (horizontal overflow, tap targets).

Known limitation: the refresh cookie rotates on every full page load; aborting a load mid-flight (very fast navigations) can lose the rotated cookie and sign the user out.

## Importing the Excel

`import-xlsx` (`backend/cmd/import-xlsx`) loads the legacy workbook (`inventario_iskra (2).xlsx`, gitignored) through the same catalog, stock and maintenance services as the API. It is a **dry-run by default**: it writes nothing and produces a review report; only `--apply` writes.

```sh
# 1. Dry-run: offline, or read-only against the database when DATABASE_URL is set
#    (the compose service sets it; start the API once first so migrations ran).
docker compose --profile tools run --rm -T importer --file /data/inventario.xlsx > import-report.md

# 2. Review import-report.md, then apply (acts as an existing active admin/operator).
docker compose --profile tools run --rm -T importer --file /data/inventario.xlsx \
  --apply --user-email you@example.com > import-report-applied.md
```

The workbook is mounted read-only from `${IMPORT_FILE:-./inventario_iskra (2).xlsx}`; set `IMPORT_FILE` to use another path. Without Docker: `go run ./cmd/import-xlsx --file <path> [flags]` from `backend/` (`DATABASE_URL` is only needed for `--apply`).

| Flag | Meaning |
| --- | --- |
| `--file` | Path of the `.xlsx` (required). |
| `--apply` | Write to the database. Requires `--user-email` and `DATABASE_URL`. |
| `--user-email` | Active admin or operator used as maintenance reporter and for opening movements. |
| `--default-location` | Location of the items of sheets 1 and 3 (default `Almacén principal`; empty means none). |
| `--report`, `--format` | Write the report to a file instead of stdout; `md` (default) or `json`. |
| `--timezone` | IANA zone for dates (default: the local zone); dates become midnight of that day. |

What it does:

- Sheets are recognized by name and headers (not by row numbers). Legacy codes are kept; rows without a code get a generated code from their category prefix (`MAQ`, `HER`, `ELE`, `CON`). Categories (`Maquinaria`, `Herramientas`, `Electricidad`, `Consumibles`) and the locations (`Maleta y caja de Alejandro` for sheet 2, the default location for sheets 1 and 3) are created when missing.
- Sheet 2 consumables (everything after the `CONSUMIBLES` marker, plus uncoded rows) become consumables with unit `unidad` and an opening movement; coded tool rows become assets. A cell with code and description fused is split. Exact repeated blocks are skipped and listed.
- `LLENO`/`VACIO` on cylinders becomes `attributes.nivel`; `Disponibilidad` is not imported (the system derives it) but contradictions are reported. Excel serial, date and `dd/mm/yyyy` values become timestamps (`last_maintenance_at`, failure dates).
- Sheet 3 items become assets with an open corrective maintenance record (status `in_repair` when the destination mentions "taller", otherwise `inoperative`).
- Every item gets `attributes.import_source = xlsx:<sheet>:r<row>`. Re-running `--apply` is safe: rows whose code (or source, for uncoded rows) already exist are reported as already imported, and a repair closed since the first run is never reopened. A code that belongs to a different existing item is reported as a conflict and skipped, never renumbered.
- Rows are written one by one (no single transaction): failures are listed in the report and the exit code is non-zero; re-running resumes safely.

The report lists totals per sheet, categories and locations to create, every item with its action and flags, one section per issue class, and a review checklist. Reports are gitignored (`import-report*`, `*.report.md`).

## Test

```sh
make -C backend test lint
# PostgreSQL integration tests are skipped unless TEST_DATABASE_URL points at a disposable database:
TEST_DATABASE_URL='postgres://postgres:test@localhost:55432/inventario_test?sslmode=disable' make -C backend test
cd frontend && npm run lint && npm run typecheck && npm test -- --run && npm run build
cd frontend && npm run e2e   # real stack, needs docker + go + chromium
```
