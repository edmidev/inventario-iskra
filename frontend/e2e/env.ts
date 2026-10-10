/** Shared constants for the e2e stack. Everything here is throwaway test data, never real secrets. */
export const PG_CONTAINER = "inv-test-pg";
export const PG_PORT = 55432;
export const API_PORT = 18080;
export const WEB_PORT = 14173;

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
  borrower: "Juan Gómez E2E",
} as const;
