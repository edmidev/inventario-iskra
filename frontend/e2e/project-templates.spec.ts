import { expect, test } from "@playwright/test";
import { ADMIN, API_URL, VIEWER } from "./env";
import { login } from "./helpers";

test("administrator registers a customer and a project with exact inventory needs", async ({ page, request }) => {
  const signed = await request.post(`${API_URL}/api/v1/auth/login`, { data: { email: ADMIN.email, password: ADMIN.password } });
  expect(signed.ok()).toBeTruthy();
  const headers = { Authorization: `Bearer ${(await signed.json()).access_token}` };
  const call = (path: string) => `${API_URL}/api/v1${path}`;
  const supplies = await request.get(call("/items?kind=consumable"), { headers });
  expect(supplies.ok()).toBeTruthy();
  const material = (await supplies.json()).data[0];
  const assets = await request.get(call("/items?kind=asset"), { headers });
  const tool = (await assets.json()).data[0];
  const suffix = String(Date.now());
  const customerName = `Cliente fabricación ${suffix}`;
  const customer = await request.post(call("/customers"), { headers, data: { name: customerName, identification: "J123", contact: "Ana", notes: "Pedido de marmita" } });
  expect(customer.status()).toBe(201);
  const projectName = `Marmita ${suffix}`;
  const project = await request.post(call("/projects"), { headers, data: { name: projectName, description: "Acero inoxidable", characteristics: ["50 litros"], estimated_days: 4, consumables: [{ item_id: material.id, quantity: 1.125 }], tools: [tool.id] } });
  expect(project.status()).toBe(201);
  expect((await project.json()).consumables[0].quantity).toBe(1.125);

  const createdItem = await request.post(call("/items"), { headers, data: { description: `Lámina fraccionada ${suffix}`, category_id: material.category_id, kind: "consumable", unit: "m" } });
  expect(createdItem.status()).toBe(201);
  const item = await createdItem.json();
  const moved = await request.post(call(`/items/${item.id}/movements`), { headers, data: { type: "in", qty: 1.125, location_id: material.stocks[0].location_id } });
  expect(moved.status()).toBe(201);
  const stored = await request.get(call(`/items/${item.id}`), { headers });
  expect((await stored.json()).stock_current).toBe(1.125);

  await login(page, "admin");
  await page.goto("/proyectos");
  await page.getByLabel("Buscar proyectos").fill(projectName);
  await page.getByRole("link", { name: projectName, exact: true }).click();
  await expect(page.getByRole("heading", { name: projectName })).toBeVisible();
  await expect(page.getByText("50 litros", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Consumibles por unidad")).toContainText("1.125");
  await page.goto("/clientes");
  await page.getByLabel("Buscar clientes").fill(customerName);
  await expect(page.getByText(customerName, { exact: true })).toBeVisible();

  const viewer = await request.post(call("/auth/login"), { data: { email: VIEWER.email, password: VIEWER.password } });
  const viewerHeaders = { Authorization: `Bearer ${(await viewer.json()).access_token}` };
  expect((await request.get(call("/projects"), { headers: viewerHeaders })).status()).toBe(403);
  expect((await request.get(call("/customers"), { headers: viewerHeaders })).status()).toBe(403);
});
