import { expect, test } from "@playwright/test";
import { ADMIN, API_URL } from "./env";
import { login } from "./helpers";

test("administrator changes worker grants without renewing the worker token", async ({ page, request }) => {
  const signedIn = await request.post(`${API_URL}/api/v1/auth/login`, { data: { email: ADMIN.email, password: ADMIN.password } });
  expect(signedIn.ok()).toBeTruthy();
  const adminSession = await signedIn.json();
  const adminHeaders = { Authorization: `Bearer ${adminSession.access_token}` };
  const worker = { name: "Permission test worker", email: `worker-${Date.now()}@e2e.test`, password: "worker-test-password-1", role: "worker" };
  const created = await request.post(`${API_URL}/api/v1/users`, { headers: adminHeaders, data: worker });
  expect(created.status()).toBe(201);
  const workerLogin = await request.post(`${API_URL}/api/v1/auth/login`, { data: { email: worker.email, password: worker.password } });
  expect(workerLogin.ok()).toBeTruthy();
  const workerSession = await workerLogin.json();
  const workerHeaders = { Authorization: `Bearer ${workerSession.access_token}` };
  const inventory = () => request.get(`${API_URL}/api/v1/items`, { headers: workerHeaders });
  expect((await inventory()).status()).toBe(403);

  try {
    await login(page, "admin");
    await page.goto("/admin/roles");
    const workerForm = page.getByRole("form", { name: "Permisos de Trabajador" });
    const read = workerForm.getByRole("checkbox", { name: "Consultar inventario" });
    await read.check();
    await workerForm.getByRole("button", { name: "Guardar permisos", exact: true }).click();
    await expect(workerForm.getByText("Permisos guardados.")).toBeVisible();
    expect((await inventory()).status()).toBe(200);
    const adminForm = page.getByRole("form", { name: "Permisos de Administrador" });
    await expect(adminForm.getByRole("checkbox", { name: /Administrar usuarios/ })).toBeDisabled();
    await expect(adminForm.getByRole("checkbox", { name: /Configurar roles/ })).toBeDisabled();
    await read.uncheck();
    await workerForm.getByRole("button", { name: "Guardar permisos", exact: true }).click();
    await expect(workerForm.getByText("Permisos guardados.")).toBeVisible();
    expect((await inventory()).status()).toBe(403);
  } finally {
    const cleared = await request.put(`${API_URL}/api/v1/roles/worker/permissions`, { headers: adminHeaders, data: { permissions: [] } });
    expect(cleared.ok()).toBeTruthy();
  }
});
