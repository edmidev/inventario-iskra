import { expect, test, type Page } from "@playwright/test";
import { SEED } from "./env";
import { choose, login } from "./helpers";

const DESCRIPTION = "Cinta de enmascarar E2E";
const unit = "rollo";
const shelf = `${SEED.warehouse} › ${SEED.shelf}`;

async function breakdown(page: Page) {
  return page.getByRole("region", { name: "Stock por ubicación" });
}

/** Reads the quantity shown for a location in the "Stock por ubicación" section. */
async function expectRow(page: Page, location: string, quantity: number | null) {
  const rows = (await breakdown(page)).getByRole("listitem");
  if (quantity === null) {
    await expect(rows.filter({ hasText: location })).toHaveCount(0);
  } else {
    await expect(rows.filter({ hasText: location })).toContainText(`${quantity} ${unit}`);
  }
}

test("a consumable stocked in several locations: create, salida, partial transfer, alerts", async ({ page }) => {
  await login(page, "operator");

  // Create it with an opening balance in two locations (5 + 5, minimum 6).
  await page.goto("/materiales");
  await page.getByRole("button", { name: /Nuevo/ }).click();
  const form = page.getByRole("dialog", { name: "Nuevo material" });
  await form.locator("label", { hasText: "Consumible" }).click();
  await form.getByLabel(/Descripción/).fill(DESCRIPTION);
  await choose(page, /Categoría/, "Consumibles");
  await form.getByLabel(/Unidad/).fill(unit);
  await form.getByLabel("Stock mínimo").fill("6");
  await form.getByRole("button", { name: "Añadir ubicación" }).click();
  await choose(page, "Ubicación del stock 1", new RegExp(`^${SEED.warehouse}$`));
  await form.getByLabel("Cantidad del stock 1").fill("5");
  await form.getByRole("button", { name: "Añadir ubicación" }).click();
  await choose(page, "Ubicación del stock 2", new RegExp(`${shelf}$`));
  await form.getByLabel("Cantidad del stock 2").fill("5");
  await expect(form.getByText(/Total:/)).toContainText(`10 ${unit}`);
  await form.getByRole("button", { name: "Crear material" }).click();

  // The new detail shows the per-location breakdown and the total.
  const section = await breakdown(page);
  await expect(section).toBeVisible();
  await expectRow(page, SEED.warehouse, 5);
  await expectRow(page, SEED.shelf, 5);
  await expect(section.getByText("Total").locator("..")).toContainText(`10 ${unit}`);

  // Salida of 3 at the shelf only.
  await page.getByRole("button", { name: "Salida", exact: true }).click();
  const out = page.getByRole("dialog", { name: "Salida de existencias" });
  await choose(page, /Ubicación de la salida/, new RegExp(SEED.shelf));
  await out.getByLabel(/Cantidad que sale/).fill("3");
  await expect(out.getByText(/Quedarán/)).toContainText(`2 ${unit} en ${shelf}`);
  await out.getByRole("button", { name: "Registrar salida" }).click();
  await expect(page.getByText(`Salida registrada en ${shelf}. Existencias: 7 ${unit}`, { exact: true })).toBeVisible();
  await expect(out).toHaveCount(0);
  await expectRow(page, SEED.warehouse, 5);
  await expectRow(page, SEED.shelf, 2);

  // Another salida above what the shelf holds is blocked.
  await page.getByRole("button", { name: "Salida", exact: true }).click();
  await choose(page, /Ubicación de la salida/, new RegExp(SEED.shelf));
  await page.getByRole("dialog").getByLabel(/Cantidad que sale/).fill("3");
  await expect(page.getByRole("dialog").getByText(/La salida supera las existencias/)).toBeVisible();
  await expect(page.getByRole("dialog").getByRole("button", { name: "Registrar salida" })).toBeDisabled();
  await page.getByRole("dialog").getByRole("button", { name: "Cancelar" }).click();

  // Partial transfer of 2 from the warehouse to a third location; the total stays at 7.
  await page.getByRole("button", { name: "Trasladar", exact: true }).click();
  const move = page.getByRole("dialog", { name: "Trasladar existencias" });
  await choose(page, /Ubicación de origen/, new RegExp(`^${SEED.warehouse} · 5`));
  await choose(page, /Ubicación de destino/, SEED.borrower);
  await move.getByLabel(/Cantidad a trasladar/).fill("2");
  await expect(move.getByText(`Mover 2 ${unit} de ${SEED.warehouse} a ${SEED.borrower}`)).toBeVisible();
  await move.getByRole("button", { name: "Trasladar", exact: true }).click();
  await expect(page.getByText(`Trasladado: 2 ${unit} de ${SEED.warehouse} a ${SEED.borrower}.`)).toBeVisible();
  await expectRow(page, SEED.warehouse, 3);
  await expectRow(page, SEED.shelf, 2);
  await expectRow(page, SEED.borrower, 2);
  await expect(section.getByText("Total").locator("..")).toContainText(`7 ${unit}`);

  // The history names where each movement happened.
  await page.getByRole("tab", { name: "Historial" }).click();
  const history = page.getByRole("list", { name: "Historial del material" });
  await expect(history).toContainText(`-3 ${unit} en ${shelf}`);
  await expect(history).toContainText(`2 ${unit}`);
  await expect(history).toContainText(`${SEED.warehouse} → ${SEED.borrower}`);
  await expect(history).toContainText(`+5 ${unit} en ${SEED.warehouse}`);

  // Salida of 2 at the third location puts the total (5) under the minimum (6): the alert lists the breakdown.
  await page.getByRole("tab", { name: "Info" }).click();
  await page.getByRole("button", { name: "Salida", exact: true }).click();
  await choose(page, /Ubicación de la salida/, new RegExp(SEED.borrower));
  await page.getByRole("dialog").getByLabel(/Cantidad que sale/).fill("2");
  await page.getByRole("dialog").getByRole("button", { name: "Registrar salida" }).click();
  await expect(page.getByText(/^Salida registrada en .*Existencias: 5 rollo$/)).toBeVisible();
  await expectRow(page, SEED.borrower, null);

  await page.goto("/stock-bajo");
  const alert = page.getByRole("listitem").filter({ hasText: DESCRIPTION });
  const places = alert.getByRole("list", { name: "Existencias por ubicación" });
  await expect(places).toContainText(`${SEED.warehouse} 3 ${unit}`);
  await expect(places).toContainText(`${SEED.shelf} 2 ${unit}`);

  // Restock from the alert: with two locations the sheet asks where.
  await alert.getByRole("button", { name: /^Entrada de/ }).click();
  const restock = page.getByRole("dialog", { name: "Entrada de existencias" });
  await expect(restock.getByRole("combobox", { name: /Ubicación/ })).toContainText("Elige una ubicación");
  await choose(page, /Ubicación/, new RegExp(`^${SEED.warehouse} · 3`));
  await restock.getByLabel(/Cantidad que entra/).fill("4");
  await restock.getByRole("button", { name: "Registrar entrada" }).click();
  await expect(page.getByText(`Entrada registrada en ${SEED.warehouse}. Existencias: 9 ${unit}`, { exact: true })).toBeVisible();
  // 9 is above the minimum of 6, so the alert is gone.
  await expect(page.getByRole("listitem").filter({ hasText: DESCRIPTION })).toHaveCount(0);
});

test("viewers see the stock per location without any action", async ({ page }) => {
  await login(page, "viewer");
  await page.goto("/materiales");
  await page.getByLabel("Buscar materiales").fill(SEED.consumable.code);
  await page.getByRole("link", { name: new RegExp(SEED.consumable.code) }).click();
  await expect(page.getByRole("region", { name: "Stock por ubicación" })).toBeVisible();
  await expect(page.getByRole("region", { name: "Acciones" })).toHaveCount(0);
});
