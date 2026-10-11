/** Shared constants for the e2e stack. Everything here is throwaway test data, never real secrets. */
export const PG_CONTAINER = "inv-test-pg";

/** Ports can be moved when the defaults are taken (e.g. a local compose stack): E2E_DB_PORT, E2E_API_PORT, E2E_WEB_PORT. */
function port(name: string, fallback: number): number {
  const raw = process.env[name];
  if (raw === undefined || raw === "") return fallback;
  const value = Number(raw);
  if (!Number.isInteger(value) || value < 1 || value > 65535) throw new Error(`${name} must be a port number, got "${raw}"`);
  return value;
}

export const PG_PORT = port("E2E_DB_PORT", 55432);
export const API_PORT = port("E2E_API_PORT", 18080);
export const WEB_PORT = port("E2E_WEB_PORT", 14173);

export const API_URL = `http://127.0.0.1:${API_PORT}`;
export const WEB_URL = `http://127.0.0.1:${WEB_PORT}`;
export const DATABASE_URL = `postgres://inventario:inventario@127.0.0.1:${PG_PORT}/inventario?sslmode=disable`;

export const ADMIN = { email: "admin@e2e.test", password: "e2e-admin-password-1", name: "Administrator" };
export const OPERATOR = { email: "operador@e2e.test", password: "e2e-operator-password-1", name: "Olga Operadora" };
export const VIEWER = { email: "lector@e2e.test", password: "e2e-viewer-password-1", name: "Luis Lector" };

/** Items created by the seed. Each spec that mutates data owns its own item. */
export const SEED = {
  readOnlyAsset: { code: "ISK-HER-001", description: "Taladro inalámbrico E2E" },
  loanAsset: { code: "ISK-HER-002", description: "Sierra circular E2E" },
  failureAsset: { code: "ISK-HER-003", description: "Lijadora orbital E2E" },
  qrAsset: { code: "ISK-HER-004", description: "Nivel láser E2E" },
  consumable: { code: "ISK-CON-001", description: "Guantes de nitrilo E2E", stock: 10, min: 5, unit: "caja" },
  /** Locations created by the seed (the consumable above starts in the warehouse). */
  warehouse: "Bodega E2E",
  shelf: "Estante A",
  borrower: "Juan Gómez E2E",
} as const;
