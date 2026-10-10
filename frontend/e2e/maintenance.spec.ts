import { expect, test } from "@playwright/test";
import { SEED } from "./env";
import { login, openItem } from "./helpers";

test("report a failure, see it under Mantenimiento, then close it", async ({ page }) => {
  const { code } = SEED.failureAsset;
  await login(page, "operator");
  await openItem(page, code);

  await page.getByRole("button", { name: "Reportar falla" }).click();
  const report = page.getByRole("dialog", { name: "Reportar falla" });
  await report.getByLabel(/¿Qué falló\?/).fill("No enciende al conectar la batería");
  await report.getByRole("button", { name: "Reportar falla" }).click();
  await expect(page.getByText(`Falla reportada en ${code}.`, { exact: true })).toBeVisible();
  await expect(page.getByText("Inoperativo").first()).toBeVisible();

  await page.goto("/mantenimiento");
  const open = page.getByRole("list", { name: "Fallas abiertas" });
  await expect(open).toContainText(code);
  await expect(open).toContainText("No enciende al conectar la batería");

  await page.getByRole("button", { name: `Cerrar falla de ${code}` }).click();
  const close = page.getByRole("dialog", { name: "Cerrar falla" });
  await close.getByLabel(/Costo/).fill("150.50");
  await close.getByRole("button", { name: "Cerrar falla" }).click();
  await expect(page.getByText("No hay fallas abiertas")).toBeVisible();

  await openItem(page, code);
  await expect(page.getByText("Funcional").first()).toBeVisible();
});
