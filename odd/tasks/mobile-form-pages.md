# Mobile form pages
Objective: replace modal and inline forms with navigable phone-friendly pages. Branch: `feat/manufacturing-03-decimals`; delivery: reviewable local commits; runner: frontend Vitest, typecheck, lint, build.

## Specs
- S1 User: "no me gustan los modales, cada formulario levantalo en una pantalla nueva, en una ruta nueva con una manera facil de retroceder, piensa como si fuese una aplicacion de telefono". Convert modal and inline action forms to route-backed screens with an obvious return path; preserve validation, data, permissions, and successful workflows. Existing forms already on pages remain there.
- S2 Assumption: confirmation-only dialogs and read-only receipt/QR views are not form pages; preserve them unless otherwise requested. Direct links and reloads must load the relevant record or show an error, without relying solely on navigation state.

## Tasks
- [x] T1 (S1,S2) Customer create/edit pages and phone-form navigation — route: inline; risk: item 2 (direct-route permission guard); commit: pending.
- [ ] T2 (S1,S2) Project and supplier create/edit pages — route: inline; risk: none; commit: pending.
- [ ] T3 (S1,S2) Materials and stock/custody/maintenance operation pages — route: inline; risk: item 3 where shared operation contracts change; commit: pending.
- [ ] T4 (S1,S2) Purchasing creation/edit/receipt pages, preserve reorder prefill — route: inline; risk: item 3 where existing navigation contracts change; commit: pending.
- [ ] T5 (S1,S2) User/category/location administration form pages — route: inline; risk: item 2 permission guards; commit: pending.
- [ ] T6 (S1,S2) Execution registration and inline action forms as pages — route: inline; risk: item 3 where shared operation contracts change; commit: pending.
- [ ] T7 (S1,S2) Feature-wide mobile navigation, access, direct entry and regression checks — route: inline, independent verifier only for named risks; commit: pending.

## Log
- L1 2026-11-11 User request: "Perfecto! otra cosa, no me gustan los modales, cada formulario levantalo en una pantalla nueva, en una ruta nueva con una manera facil de retroceder, piensa como si fuese una aplicacion de telefono".
- L2 Read-only mapping found forms in modals across customers, projects, suppliers, inventory, purchasing and administration, plus inline execution forms. Reorder suggestions pass prefilled purchase values; preserve them. Existing unrelated dirty changes, including the More menu and purchase order detail, must not be overwritten.
- L3 2026-11-11 T1 moves customer creation/editing to /clientes/nuevo and /clientes/:id/editar, with direct ID loading, guarded access, deterministic return and preservation of list filters. RED→GREEN observed. Independent verifier PASS for unauthorized direct edit, missing customer and cancellation; its three probes were adopted as regression tests. Parent focused suite: 9 tests pass; typecheck/lint pass; earlier full frontend suite: 277 tests and build pass before adopting three verifier probes. Browser visual QA pending. Commit pending.
