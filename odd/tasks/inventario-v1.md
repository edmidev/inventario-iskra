# Feature: inventario-v1

Plan source: `/home/edgar/.claude/plans/tenemos-que-desarrollar-un-misty-panda.md`

## Objective
Mobile-first inventory system (Go backend, React frontend) replacing `inventario_iskra (2).xlsx`: tracks assets and consumables, roles, basic custody (loan/return), maintenance.

## Constraints
- Multi-user with roles (admin/operator/viewer) from v1. Basic custody in v1.
- Hexagonal/screaming backend modules; atomic design + container/presentational frontend.
- Artifacts in English. Conventional Commits, no AI attribution.
- ~400 changed lines per task is a planning heuristic only.

## Resolved config
- TDD: unknown/not configured -> ordinary functional checks (`go test ./...`, `npm test`). Source: none set.
- RDD: not enabled by user (off by default).
- Delivery strategy: ask-on-risk (default).
- Toolchain: go 1.27.1, node 22.20, docker, psql 17, git 2.47.

## Tasks
- [x] T0 Scaffold monorepo, Docker Compose, CI, OpenAPI base (route: delegated writer; trigger: 2+ non-trivial files)
- [x] T1 identity: login, roles, user management
- [x] T2 catalog: categories, locations, items CRUD, search/filters, code generator
- [x] T3 stock: movements, consumable stock, min-stock alerts
- [x] T4 custody: loan/return with history
- [x] T4b follow-up: stock `transfer` must reject assets on loan (`item_on_loan`), found by T4 writer (route: folded into T5 writer's first work unit)
- [x] T5 maintenance: records, failure report, due dates
- [x] T6 frontend: dashboard, PWA, QR (split, one writer at a time, backend first)
  - [x] T6a backend `GET /dashboard/summary` (+ fix stale README line)
  - [x] T6b frontend foundation: API client, auth + role guards, layout, dashboard, items list/detail/create/edit
  - [x] T6c frontend operations: stock movements + alerts, loan/return, maintenance flows, history tabs
  - [x] T6d frontend QR scan/labels, admin (users, categories, locations), PWA, Playwright mobile e2e
- [ ] T6e follow-up (backend): refresh-token rotation race signs users out when a page load aborts after rotation; add a short reuse grace window or idempotent rotation (found by T6d)
- [x] T7 xlsx importer + real data migration
- [ ] T8 stock per location for consumables (new requirement from the user: the same consumable can be split across locations, e.g. 5 in "Maleta y caja de Alejandro" and 5 in the workshop)
  - [x] T8a backend: `item_stocks(item_id, location_id, quantity)`, movements carry a location, partial-quantity transfer between locations, `stock_current` = sum, alerts on total, dashboard/API shape additive, backfill from current single location
  - [ ] T8b frontend: per-location breakdown on consumable detail, location pickers in in/out/adjust, transfer with quantity
  - [ ] T8c importer: sheet 2 rows 87-123 are NOT duplicates; import them as stock in a second location (blocked on which block belongs to which location)

## Acceptance
Per plan "Verificacion" section.

## Progress / evidence
- Created feature doc; repo initialized on branch `feat/inventario-v1`.
- T0 done (delegated writer). Commits: 904fa3d, 74ba9c9, 3926ca0, 4374ac4. Checks observed: `go vet` + `go test ./...` ok; frontend vitest 1/1 + build ok; `docker compose config -q` ok. Parent spot check re-ran go test and vitest: pass. Not verified: docker image builds, `compose up`. RDD off; no review.
- T1 done (delegated writer). Commits: 7968566, d8d473c, f3ad735, 98fad5d. Parent spot check with throwaway postgres:16: go vet + go test ./... -p 1 all ok incl. Postgres integration tests. Deviations: plain pgx instead of sqlc (sqlc.yaml removed); `.env.example` not updated (access denied by a deny rule; user must add JWT_SECRET, COOKIE_SECURE, BOOTSTRAP_ADMIN_EMAIL, BOOTSTRAP_ADMIN_PASSWORD). Known: access JWT valid up to 15m after deactivation; rate limiter in-memory; Postgres tests need `-p 1`.
- T2 done (delegated writer). Commits: bc943b7, 6d8bae4, f94d219, 808808e, 3ae4afb. Parent spot check with throwaway postgres:16: gofmt clean, go vet ok, go test ./... -p 1 all ok incl. catalog Postgres tests. Notes: validation errors are 422 with fields; availability derived via domain.DeriveAvailability (T4 fills ActiveLoan); only admin may supply a manual code; category/location ids are numeric, items uuid; location cycle check not race-safe (admin-only); no optimistic locking on item update.
- T3 done (delegated writer). Commits: 635b752, e1e41e1, ecb2e80, bce3249, 46fda31. Parent spot check with throwaway postgres:16: gofmt clean, go vet ok, go test ./... -p 1 all ok incl. stock Postgres tests. Notes: movements types opening|in|out|adjust|transfer; stock_movements.user_id nullable (system/backfill); opening movement recorded via catalog adapter create-hook wired in cmd/api/wire.go (same tx); TRUNCATE not blocked by immutability trigger; adjust to same count records delta 0.
- T4 done (delegated writer). Commits: dd4d15c, d3abf65, 57d8c3d, c671e67, d4fddf3, a34cf50. Parent spot check with throwaway postgres:16: gofmt clean, go vet ok, go test ./... -p 1 all ok incl. custody Postgres tests. Notes: catalog adapter LEFT JOINs custody_assignments (adapter-level coupling only); items expose on_loan + custody; catalog rejects delete/location change while on loan; assignee FK RESTRICT so locations with custody history cannot be deleted; overdue uses app clock. OPEN HOLE: stock transfer can still move an asset on loan (T4b).
- T4b + T5 done (one delegated writer). Commits: 117a092, b6d33df, 0c92ac1, e937f41, e0a8c8d, aecca9b, da3b2ce. Parent spot check with throwaway postgres:16: gofmt clean, go vet ok, go test ./... -p 1 all 24 packages ok. Notes: `/maintenance/due` within_days 0..365 default 0 (overdue only); last_maintenance_at never moves backwards; plain PATCH /items can still change status while a failure is open (unguarded); costs float64 rounded to 2 decimals at API edge; catalog DTO overdue uses time.Now(). No dashboard endpoint and no CORS exist yet (web uses nginx /api proxy; dev needs Vite proxy).
- T6a done (delegated writer). Commits: b28ad90, 0ff3d08, eb8f70b. Parent spot check with throwaway postgres:16: gofmt clean, go vet ok, go test ./... -p 1 all 26 packages ok, none failing. Added GET /dashboard/summary (internal/dashboard) and 409 `item_under_repair` guard on PATCH status during open failure.
- T6b done as `partial` (delegated writer; no browser available so nothing checked visually). Commits: 7d8bd67, d4fd77e, 25ded78, 9a6c4a9, 830786c, 9039368. Parent spot check: tsc clean, oxlint clean, vitest 8 files / 59 tests pass, vite build ok (single ~596 kB chunk warning). Real backend smoke through Vite proxy passed for login/refresh/dashboard/catalog/items; custody/UI flows only MSW-tested. Notes: oxlint replaces eslint (TS 7 incompatible with typescript-eslint); `API_PROXY_TARGET` override added in vite.config.ts (port 8080 on this machine is held by an unrelated process); intermediate commits 25ded78 and 9a6c4a9 not individually green (final tree green). API gaps: Item lacks category/location names, no on-loan filter, no item location history endpoint. Visual mobile check still pending (do in T6d Playwright + Chrome).
- T6c done as `partial` (delegated writer, resumed once after a rate-limit stop; no browser available so nothing checked visually). Commits: 014fb14, 4262e96, 9b6f946, 20ece9c, 6aba753, db736e7, a019012. Parent spot check: tsc clean, oxlint clean, vitest pass (agent reported 14 files / 103 tests), vite build ok, entry chunk 595.88 -> 461.58 kB. Real-API HTTP round trips for stock/custody/maintenance/dashboard parsed with the app's zod schemas (19 payloads); UI flows only MSW-tested. API gaps: openapi Error.code omits new 409 codes; Item lacks loan origin and open_failure flag; no currency defined. Visual mobile check still pending (T6d).
- T6d done (delegated writer). Commits: 9059ca4, 21122f1, ff6affb, d90fe18, a49887f, e64fb2c, 52cce02, b177f1a. Parent spot check: tsc clean, oxlint clean, vitest 22 files / 158 tests pass, vite build ok (entry chunk 319.58 kB; qr-scanner lazy chunks). Agent reported e2e 21/21 pass at 390x844 on a real stack (not re-run by parent) and a Playwright screenshot review in light/dark (about 16 of 18 screens viewed; fixes for focus tooltip, placeholders, tap targets, scan layout). Not verified: real phone camera, print preview, PWA install prompt, iOS Safari, CI e2e job on GitHub. Behavior fixes: offline session state `unreachable`, react-query networkMode always. Backend gaps: refresh rotation race (T6e), GET /users has no pagination/filters, users validation errors are 400 without fields, Item lacks category/location names and on-loan filter.
- T7 done (delegated writer). Commits: 73721fc, e2dce88, 7c36a67, e0d5efc, da0c5bf, 1d12472. Parent spot check with throwaway postgres:16: gofmt clean, go vet ok, go test ./... -p 1 all packages ok, 9 importer tests pass incl. Postgres ones. Dry-run on the real workbook (offline): 158 rows read, 125 to create (85 assets, 40 consumables), 33 exact-repeat rows skipped (sheet 2 rows 87-123), 0 conflicts, 0 invalid, 31 generated codes, 2 open failure records (ISK-HER-013 in_repair, ISK-HER-014 inoperative). Agent applied twice on a throwaway DB: run 2 created 0. NOT applied to the user's database yet: awaiting the user's review of the dry-run report. Open questions: default location, duplicate block confirmation, ISK-TALBA-015 VACIO, ISK-TALELC-016 loan note, failures for ISK-CORTPLAS-005/ISK-COMPR-006, generated codes for uncoded rows, time zone. Note: at the time of the check the user's compose stack (containers and volume inventario_db-data) no longer existed; cause not established (it only held the bootstrap admin).
- Requirement change (user): rows 87-123 of the Excel sheet 2 are the same consumables stocked in a second location, not a paste duplicate. Importer apply is on hold until T8 is decided. User confirmed: rows 44-86 = Maleta y caja de Alejandro, rows 87-123 = taller (quantities as written).
- T8a done (delegated writer). Commits: a064f2a, 7fc2b7a. Parent spot check with throwaway postgres:16: gofmt clean, go vet ok, go test ./... -p 1 all 28 packages ok (agent reported 264 PASS / 0 SKIP incl. migration backfill, invariants, concurrency); frontend tsc clean + 158 tests pass (no frontend edits). Agent smoke on a throwaway API passed; e2e skipped because the user's compose stack holds port 18080. Decisions: zero-quantity rows deleted; location inference when omitted (single row, else latest movement location, else `Sin ubicación`, ambiguous => 422 `location_required`); consumables' items.location_id is NULL; consumable transfer now REQUIRES qty > 0 (existing frontend Traslado for consumables must be updated in T8b); movements carry location_id and moved_qty; alerts/dashboard carry `stocks`. Risks: legacy history replay assumes consistency; migration Down refuses with multi-location openings.

## Next step
T8b frontend per-location stock, then T8c importer (block 44-86 = maleta, 87-123 = taller, confirmed by the user) + dry-run review + apply; T6e refresh-token race fix.
