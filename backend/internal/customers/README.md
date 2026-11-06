# Customer catalog

Customers retain name, identification, contact, notes and an active flag. Only the name is required; fields are trimmed and bounded. Creation defaults to active. Deactivation keeps the customer readable; deletion is soft, retaining the row for future execution and delivery references.

Wire `app.NewService(postgres.New(pool))` into `httpapi.New(service, authenticate, httpapi.Permissions{Read: "customers:read", Write: "customers:write"})`, then `Register(router)`. API: paged/filterable `GET /customers?q=&active=&limit=&offset=`, `POST /customers`, `GET /customers/{id}`, `PATCH /customers/{id}` and `DELETE /customers/{id}`. PATCH preserves unspecified fields; `active: false` is explicit.

Permission configuration and route composition belong to the parent application. The customer catalog alone does not create executions or deliveries, and it does not assign finished products to customers.
