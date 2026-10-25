# Feature: purchasing

Objective: suppliers and purchase orders (with receipts) in the inventory system. Branch: `feat/purchasing`. Delivery: ask-on-risk. Test runner: `go test ./... -p 1` (Postgres integration tests need a throwaway postgres), `npm test` in `frontend/`.

## Specs
- S1: "ordenes de compra en el inventario, manejar proveedores" — suppliers module (CRUD, search, pagination, soft delete, active flag; admin/operator write, viewer read).
- S2: Purchase orders with states `draft -> ordered -> partially_received -> received` plus `cancelled`; lines with ordered/received quantity and unit cost; partial receipts with a destination location.
- S3: "Si el articulo no existe, que se pueda crear dentro de la orden de compra" — a line references an existing item OR carries a new-item definition (validated on save, created at first receipt in the same transaction through the catalog service).
- S4: "Que se creen N ítems con códigos distintos" — a new asset line with quantity N creates N items with distinct generated codes on receipt (partial receipt creates only the received count). New consumable creates one item plus an `in` movement.
- S5 (assumption): receiving never writes stock directly; it records `in` movements through the stock service in the same transaction. Cap on N per line (assumed 100).
- S6: "multimoneda, dolares y bs" — unit costs support USD and VES (Bs). Assumption: one currency per order (`currency` in `USD|VES`, all lines share it); an optional exchange rate (VES per USD) may be recorded on the order for reference, never used to convert stored amounts. Amounts stored as integer minor units (cents) to avoid float rounding.

## Tasks
- [x] T9 suppliers backend (S1) — route: inline (no writer trigger: one understood module, ~1k lines incl. tests, written sequentially); commits: 38129e0, 82ed154, 5c03956
- [x] T10a shared tx helper: `db.WithTx`/`db.Begin`; catalog item create and stock Record join an ambient tx (S5) — route: inline; commit: f939fbf
- [ ] T10b purchasing backend: orders, lines, receipts, multi-currency (S2-S6) — route: delegated writer (named reason: parent context backstop, ~150k)
- [ ] T11 alerts/dashboard integration: pending-on-order quantity, suggest PO from low stock
- [ ] T12 frontend: suppliers and orders pages, receive flow, e2e

## Log
- L1 (2026-10-25, user): "Necesito que ahora el sistema maneje ordenes de compra en el inventario, manejar proveedores, necesito que me propongas un plan"
- L2 (user): "Si el articulo no existe, que se pueda crear dentro de la orden de compra"
- L3 (user): "Que se creen N ítems con códigos distintos"
- L4 (user): "dale, arrancá con T9"

## Progress / evidence
- T9 done inline on `feat/purchasing`. Checks observed: gofmt clean, `go vet ./...` ok, `go test ./... -p 1` against a throwaway postgres:16 (container removed afterwards): all packages ok; suppliers domain/app/httpapi/postgres tests pass (the only failure was a wrong expectation in my own test, fixed). OpenAPI parses (python yaml) and lists `/suppliers`, `/suppliers/{id}`; down migration statement validated in a rolled-back transaction. Tests were written after the code, so no RED was observed.
- Decisions: suppliers use BIGINT identity ids (like categories); name unique among live suppliers via partial index on `lower(name)`; soft delete; write permission reuses `items:write` (operator, admin), read reuses `inventory:read`, so identity was not touched.
- Not done: no differential run against a base copy (only additive wiring changed: new route mount in cmd/api). Optional `item_suppliers` (habitual supplier / last price) deferred to T11.
- Risk: none (additive module, new table, one new route mount; no shared code changed beyond gofmt of wire.go imports).
- Next: T10 purchasing backend (currency decision S6 resolved, L5).
- L5 (user): "multimoneda, dolares y bs"
- L6 (user): "dale, arrancá con T10"
- T10a done inline. Checks observed: `go build`, `go vet`, gofmt clean; `go test ./... -p 1 -count=1` on throwaway postgres:16 (removed): all packages ok, incl. new `platform/db` tx tests and the unchanged catalog/stock Postgres suites (behavior preserved: outside an ambient tx `db.Begin` begins/commits exactly as before). Risk: item 3 (shared code: catalog `Items.Create` and stock `Repo.Record` now call `db.Begin`) — no behavior change without an ambient tx, covered by the existing suites; no independent verifier.
- Design for T10b (parent decisions): assets are always new-item lines (each unit is unique; existing-item lines must be consumables, frontend can prefill a new_item from an existing item); receiving creates N assets at the receipt location via the catalog service and records consumable stock with a stock `in` movement via the stock service, both inside one transaction (ambient tx); purchasing talks to catalog/stock only through ports implemented in cmd/api wiring; money as integer minor units; idempotent receive via optional `client_key`.
