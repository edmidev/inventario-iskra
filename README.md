# Inventario

Mobile-first inventory system for assets and consumables, with roles, basic custody (loan/return) and maintenance tracking.

## Stack

- Backend: Go, chi, PostgreSQL 16, goose (migrations), sqlc (pgx/v5). Hexagonal layout with one module per business area.
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
make -C backend test run    # needs DATABASE_URL for future DB features
cd frontend && npm install && npm run dev
```

## Test

```sh
make -C backend test lint
cd frontend && npm test -- --run && npm run build
```
