import { expect, test } from "@playwright/test";
import { SEED } from "./env";
import { login, openItem } from "./helpers";

test("a viewer sees no action buttons and no admin entries", async ({ page }) => {
  await login(page, "viewer");

  await page.goto("/materiales");
  await expect(page.getByRole("button", { name: "Nuevo" })).toHaveCount(0);

  await openItem(page, SEED.readOnlyAsset.code);
  for (const name of ["Prestar", "Devolver", "Trasladar", "Reportar falla", "Registrar mantenimiento", "Editar", "Eliminar"]) {
    await expect(page.getByRole("button", { name, exact: true })).toHaveCount(0);
  }
  // Viewing the label is read-only and stays available.
  await expect(page.getByRole("button", { name: "Etiqueta QR" })).toBeVisible();

  await page.goto("/mas");
  await expect(page.getByRole("heading", { name: "Más" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Administración" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: /Usuarios/ })).toHaveCount(0);

  await page.goto("/admin/usuarios");
  await expect(page.getByText("Sin acceso")).toBeVisible();
  await page.goto("/materiales/etiquetas");
  await expect(page.getByText("Sin acceso")).toBeVisible();
});

test("an operator has no admin entries either", async ({ page }) => {
  await login(page, "operator");
  await page.goto("/mas");
  await expect(page.getByRole("heading", { name: "Más" })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Administración" })).toHaveCount(0);
});
