# Reusable project templates

This module owns templates, not executions. Templates describe the materials for **one manufactured unit**, exact informational asset tools, characteristics and estimated calendar days. Quantities use `platform/quantity.Quantity`; JSON and stored template documents remain human-unit decimals.

## Integration

```go
svc := app.NewService(postgres.New(pool), itemCatalog)
handler := httpapi.New(svc, authenticate, httpapi.Permissions{
    Read: "projects:read", Write: "projects:write",
})
handler.Register(router)
```

`app.ItemCatalog.Kind(ctx, itemID)` returns `"asset"` or `"consumable"` for a live catalog item. Map a missing/deleted item to `app.ErrItemNotFound`; propagate unavailable storage and other infrastructure errors. The service validates consumable/tool kinds and duplicates before persistence.

Endpoints: `GET /projects?q=&limit=&offset=`, `POST /projects`, `GET /projects/{id}`, `PUT /projects/{id}` (full template replacement), `DELETE /projects/{id}` (soft delete). Create and replace bodies contain `name`, `description`, `characteristics: string[]`, `estimated_days`, `consumables: [{item_id, quantity}]`, `tools: string[]`. Empty lists are supported. Estimated days is a positive integer.

Names are unique among live templates, ignoring case. Each row's complete bill of materials is one atomic JSONB document; updates cannot expose a partially replaced list. Deletion retains the row. Inventory IDs remain exact references; no tool reservation or stock movement occurs. Execution creation must copy the template, never retain a live pointer to mutable lists.

Migration `00014_customer_project_templates.sql` creates these tables and the customer catalog. The parent composition root must register routes, add supported permission keys, and connect the frontend routes. The feature does not invent grants for workshop roles.

## Checks

`go test ./internal/projects/... ./internal/customers/...` covers domain/application and HTTP rules. PostgreSQL persistence test requires a **disposable** `TEST_DATABASE_URL`; it migrates that database and truncates only these new tables. Do not point it at production.
