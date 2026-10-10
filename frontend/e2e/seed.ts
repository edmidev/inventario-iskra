import { ADMIN, API_URL, OPERATOR, SEED, VIEWER } from "./env";

async function call<T>(path: string, token: string | null, init: { method?: string; body?: unknown } = {}): Promise<T> {
  const response = await fetch(`${API_URL}/api/v1${path}`, {
    method: init.method ?? (init.body ? "POST" : "GET"),
    headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: init.body ? JSON.stringify(init.body) : undefined,
  });
  if (!response.ok) throw new Error(`${init.method ?? "GET"} ${path} -> ${response.status} ${await response.text()}`);
  return response.status === 204 ? (undefined as T) : ((await response.json()) as T);
}

/** Creates the catalog, users and items every spec relies on, through the public API. */
export async function seed(): Promise<void> {
  const login = await call<{ access_token: string }>("/auth/login", null, { body: { email: ADMIN.email, password: ADMIN.password } });
  const token = login.access_token;

  await call("/users", token, { body: { ...OPERATOR, role: "operator" } });
  await call("/users", token, { body: { ...VIEWER, role: "viewer" } });

  const tools = await call<{ id: number }>("/categories", token, { body: { name: "Herramientas", code_prefix: "HER" } });
  const supplies = await call<{ id: number }>("/categories", token, { body: { name: "Consumibles", code_prefix: "CON" } });

  const warehouse = await call<{ id: number }>("/locations", token, { body: { name: "Bodega E2E", kind: "storage" } });
  await call("/locations", token, { body: { name: "Estante A", kind: "container", parent_id: warehouse.id } });
  await call("/locations", token, { body: { name: SEED.borrower, kind: "person" } });

  const assets = [SEED.readOnlyAsset, SEED.loanAsset, SEED.failureAsset, SEED.qrAsset];
  for (const asset of assets) {
    const created = await call<{ code: string }>("/items", token, {
      body: { kind: "asset", description: asset.description, category_id: tools.id, location_id: warehouse.id, maintenance_interval_days: 90 },
    });
    if (created.code !== asset.code) throw new Error(`Seed expected ${asset.code} but the API generated ${created.code}`);
  }
  const consumable = await call<{ code: string }>("/items", token, {
    body: {
      kind: "consumable",
      description: SEED.consumable.description,
      category_id: supplies.id,
      location_id: warehouse.id,
      unit: SEED.consumable.unit,
      stock_current: SEED.consumable.stock,
      stock_min: SEED.consumable.min,
    },
  });
  if (consumable.code !== SEED.consumable.code) throw new Error(`Seed expected ${SEED.consumable.code} but got ${consumable.code}`);
}
