import { expect, test } from "@playwright/test";
import { SEED } from "./env";
import { login } from "./helpers";

test("manual code entry resolves to the item detail", async ({ page }) => {
  await login(page, "operator");
  await page.getByRole("link", { name: "Escanear" }).first().click();
  await expect(page.getByRole("heading", { name: "Escanear" })).toBeVisible();
  // Headless Chromium has no camera: the page must say so and keep manual entry usable.
  await expect(page.getByRole("region", { name: "Cámara" })).toContainText(/cámara/i);

  await page.getByLabel("Código del material").fill(SEED.qrAsset.code.toLowerCase());
  await page.getByRole("button", { name: "Buscar" }).click();
  await expect(page.getByRole("heading", { name: SEED.qrAsset.code })).toBeVisible();
  await expect(page.getByText(SEED.qrAsset.description).first()).toBeVisible();
});

test("a pasted label URL resolves too, and an unknown code is reported", async ({ page }) => {
  await login(page, "viewer");
  await page.goto("/escanear");
  await page.getByLabel("Código del material").fill(`https://inventario.example.com/materiales/codigo/${SEED.qrAsset.code}`);
  await page.getByRole("button", { name: "Buscar" }).click();
  await expect(page.getByRole("heading", { name: SEED.qrAsset.code })).toBeVisible();

  await page.goto("/escanear");
  await page.getByLabel("Código del material").fill("ISK-XXX-999");
  await page.getByRole("button", { name: "Buscar" }).click();
  await expect(page.getByRole("alert").filter({ hasText: "No se encontró ningún material" })).toBeVisible();
});

test("a printed label URL opens the item directly", async ({ page }) => {
  await login(page, "viewer");
  await page.goto(`/materiales/codigo/${SEED.qrAsset.code}`);
  await expect(page.getByRole("heading", { name: SEED.qrAsset.code })).toBeVisible();
  await expect(page).toHaveURL(/\/materiales\/[0-9a-f-]{36}$/);

  await page.goto("/materiales/codigo/ISK-NOPE-001");
  await expect(page.getByText("Material no encontrado")).toBeVisible();
});

test("the item label sheet encodes the resolver URL", async ({ page }) => {
  await login(page, "operator");
  await page.goto(`/materiales/codigo/${SEED.qrAsset.code}`);
  await page.getByRole("button", { name: "Etiqueta QR" }).click();
  const dialog = page.getByRole("dialog", { name: "Etiqueta QR" });
  await expect(dialog.getByText(SEED.qrAsset.code, { exact: true })).toBeVisible();
  const value = await dialog.locator("svg[data-value]").getAttribute("data-value");
  expect(value).toBe(`${new URL(page.url()).origin}/materiales/codigo/${SEED.qrAsset.code}`);
});
