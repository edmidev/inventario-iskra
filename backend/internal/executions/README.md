# Manufacturing executions

Executions register production work against an existing project template. Each record stores an independent snapshot, one or more customers, an active principal user, and optional active workers.

## Registration

`POST /executions` accepts:

| Field | Rule |
|-------|------|
| `project_id` | Required existing project template. |
| `units` | Required positive integer; consumables remain stored as per-unit snapshot quantities. Reservation and purchasing stages derive total demand by multiplying this value by each per-unit quantity. |
| `estimated_days` | Optional positive integer; defaults to the template estimate and can be adjusted per execution. |
| `customer_ids` | Required, non-empty, unique customer IDs. Product allocation is deferred until delivery. |
| `principal_user_id` | Required active user. |
| `worker_ids` | Optional unique list of active users. |

Editing a project template later does not alter an existing execution snapshot. Tool IDs are informational and are not exclusively reserved by an execution.

## Lifecycle and access

| Operation | Route | Permission |
|-----------|-------|------------|
| List, read | `GET /executions`, `GET /executions/{id}` | `executions:read` |
| Create | `POST /executions` | `executions:write` |
| List assignable active users | `/executions/assignable-users` | `executions:write` |
| Start, pause, resume | `/executions/{id}/start`, `/pause`, `/resume` | `executions:write` |

Starting is allowed while inventory is incomplete; materials can be addressed during execution. Concurrent state transitions use conditional updates so only one competing transition succeeds.

Completion, cancellation, reservations, deliveries, and procurement are handled in later manufacturing stages.
