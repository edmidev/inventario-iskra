import { expect, test } from "@playwright/test";
import { SEED } from "./env";
import { login } from "./helpers";

test("login, search an item and open its detail", async ({ page }) => {
  await login(page, "operator");
  await page.getByRole("link", { name: "Materiales" }).first().click();
  await expect(page.getByRole("heading", { name: "Materiales" })).toBeVisible();

  await page.getByLabel("Buscar materiales").fill("taladro");
  const card = page.getByRole("link", { name: new RegExp(SEED.readOnlyAsset.code) });
  await expect(card).toBeVisible();
  await expect(page.getByRole("link", { name: new RegExp(SEED.loanAsset.code) })).toHaveCount(0);

  await card.click();
  await expect(page.getByRole("heading", { name: SEED.readOnlyAsset.code })).toBeVisible();
  await expect(page.getByText(SEED.readOnlyAsset.description).first()).toBeVisible();
  await expect(page.getByText("Disponible")).toBeVisible();
  await expect(page.getByText("Herramientas")).toBeVisible();
  await expect(page.getByText("Bodega E2E")).toBeVisible();
});

test("a wrong password shows a readable error and stays on the login page", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Correo").fill("operador@e2e.test");
  await page.getByLabel("Contraseña").fill("incorrecta-123");
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("alert")).toContainText("Correo o contraseña incorrectos.");
});

test("an unauthenticated visit to a deep link goes to login and returns after signing in", async ({ page }) => {
  await page.goto("/materiales/codigo/ISK-HER-001");
  await expect(page.getByLabel("Correo")).toBeVisible();
  await page.getByLabel("Correo").fill("operador@e2e.test");
  await page.getByLabel("Contraseña").fill("e2e-operator-password-1");
  await page.getByRole("button", { name: "Entrar" }).click();
  await expect(page.getByRole("heading", { name: "ISK-HER-001" })).toBeVisible();
});
