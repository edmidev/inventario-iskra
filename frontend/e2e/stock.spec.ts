import { expect, test } from "@playwright/test";
import { SEED } from "./env";
import { login, openItem } from "./helpers";

test("consumable salida and entrada update the stock and the low-stock list", async ({ page }) => {
  const { code, stock, unit } = SEED.consumable;
  await login(page, "operator");
  await openItem(page, code);
  await expect(page.getByText(`${stock} ${unit}`).first()).toBeVisible();

  // Salida of 6 leaves 4, under the minimum of 5.
  await page.getByRole("button", { name: "Salida", exact: true }).click();
  const out = page.getByRole("dialog", { name: "Salida de existencias" });
  // The consumable sits in one location only, so the sheet preselects it.
  await expect(out.getByRole("combobox", { name: /Ubicación de la salida/ })).toContainText(SEED.warehouse);
  await out.getByLabel(/Cantidad que sale/).fill("6");
  await out.getByRole("button", { name: "Registrar salida" }).click();
  await expect(page.getByText(`Salida registrada en ${SEED.warehouse}. Existencias: ${stock - 6} ${unit}`, { exact: true })).toBeVisible();
  await expect(page.getByText("Stock bajo").first()).toBeVisible();
  await expect(page.getByRole("list", { name: "Existencias por ubicación" })).toContainText(`${SEED.warehouse}${stock - 6} ${unit}`);

  await page.goto("/stock-bajo");
  const alerts = page.getByRole("list", { name: "Consumibles con stock bajo" });
  await expect(alerts).toContainText(code);

  // Entrada of 10 brings it back above the minimum.
  await openItem(page, code);
  await page.getByRole("button", { name: "Entrada", exact: true }).click();
  const input = page.getByRole("dialog", { name: "Entrada de existencias" });
  await input.getByLabel(/Cantidad que entra/).fill("10");
  await input.getByRole("button", { name: "Registrar entrada" }).click();
  await expect(page.getByText(`Entrada registrada en ${SEED.warehouse}. Existencias: ${stock - 6 + 10} ${unit}`, { exact: true })).toBeVisible();

  await page.goto("/stock-bajo");
  await expect(page.getByText("Todo el stock está en orden")).toBeVisible();
});

test("an exit larger than the stock is blocked with a readable message", async ({ page }) => {
  await login(page, "operator");
  await openItem(page, SEED.consumable.code);
  await page.getByRole("button", { name: "Salida", exact: true }).click();
  const out = page.getByRole("dialog", { name: "Salida de existencias" });
  await out.getByLabel(/Cantidad que sale/).fill("9999");
  await expect(out.getByText(/La salida supera las existencias de/)).toBeVisible();
  await expect(out.getByRole("button", { name: "Registrar salida" })).toBeDisabled();
});
