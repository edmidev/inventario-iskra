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
- [ ] T6 frontend: dashboard, PWA, QR (split, one writer at a time, backend first)
  - [ ] T6a backend `GET /dashboard/summary` (+ fix stale README line)
  - [ ] T6b frontend foundation: API client, auth + role guards, layout, dashboard, items list/detail/create/edit
  - [ ] T6c frontend operations: stock movements + alerts, loan/return, maintenance flows, history tabs
  - [ ] T6d frontend QR scan/labels, admin (users, categories, locations), PWA, Playwright mobile e2e
- [ ] T7 xlsx importer + real data migration

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

## Next step
T6a backend dashboard summary, then T6b frontend foundation.
