import { expect, test } from "@playwright/test";
import { choose, login } from "./helpers";

test("an admin creates a user who can then sign in", async ({ page }) => {
  await login(page, "admin");
  await page.getByRole("link", { name: "Más" }).first().click();
  await page.getByRole("navigation", { name: "Administración" }).getByRole("link", { name: /Usuarios/ }).click();
  await expect(page.getByRole("heading", { name: "Usuarios" })).toBeVisible();

  await page.getByRole("button", { name: "Nuevo" }).click();
  const dialog = page.getByRole("dialog", { name: "Nuevo usuario" });
  await dialog.getByLabel(/Nombre/).fill("Nora Nueva");
  await dialog.getByLabel(/Correo/).fill("nora@e2e.test");
  await dialog.getByLabel(/Contraseña inicial/).fill("clave-de-nora-123");
  await choose(page, /Rol/, "Operador");
  await dialog.getByRole("button", { name: "Crear usuario" }).click();
  await expect(page.getByText("Usuario Nora Nueva creado.", { exact: true })).toBeVisible();
  await expect(page.getByRole("list", { name: "Usuarios" })).toContainText("nora@e2e.test");

  // A duplicate email is reported on the field.
  await page.getByRole("button", { name: "Nuevo" }).click();
  const again = page.getByRole("dialog", { name: "Nuevo usuario" });
  await again.getByLabel(/Nombre/).fill("Otra Nora");
  await again.getByLabel(/Correo/).fill("nora@e2e.test");
  await again.getByLabel(/Contraseña inicial/).fill("clave-de-nora-123");
  await again.getByRole("button", { name: "Crear usuario" }).click();
  await expect(again.getByText("Ya existe un usuario con ese correo.")).toBeVisible();
  await again.getByRole("button", { name: "Cancelar" }).click();

  // The last-admin rule is explained, not a generic failure.
  await page.getByRole("button", { name: "Editar Administrator" }).click();
  const edit = page.getByRole("dialog", { name: "Editar usuario" });
  await edit.getByRole("checkbox", { name: /Cuenta activa/ }).uncheck();
  await edit.getByRole("button", { name: "Guardar cambios" }).click();
  await expect(edit.getByRole("alert")).toContainText("último administrador activo");
  await edit.getByRole("button", { name: "Cancelar" }).click();

  // The new user signs in as an operator (no admin entries).
  await page.getByRole("link", { name: "Más" }).first().click();
  await page.getByRole("region", { name: "Cuenta" }).getByRole("button", { name: /Cerrar sesión/ }).click();
  await page.getByLabel("Correo").fill("nora@e2e.test");
  await page.getByLabel("Contraseña").fill("clave-de-nora-123");
  await page.getByRole("button", { name: "Entrar" }).click();
  // Signing in returns to where the session ended (Más); an operator sees no admin entries there.
  await expect(page.getByRole("region", { name: "Cuenta" })).toContainText("nora@e2e.test");
  await expect(page.getByRole("navigation", { name: "Administración" })).toHaveCount(0);
});

test("an admin manages categories and locations", async ({ page }) => {
  await login(page, "admin");

  await page.goto("/admin/categorias");
  await page.getByRole("button", { name: "Nueva" }).click();
  const category = page.getByRole("dialog", { name: "Nueva categoría" });
  await category.getByLabel(/Nombre/).fill("Seguridad E2E");
  await category.getByLabel(/Prefijo/).fill("seg");
  await category.getByRole("button", { name: "Crear categoría" }).click();
  await expect(page.getByRole("list", { name: "Categorías" })).toContainText("SEG");

  // Herramientas is used by seeded items.
  await page.getByRole("button", { name: "Eliminar Herramientas" }).click();
  await page.getByRole("dialog", { name: "Eliminar categoría" }).getByRole("button", { name: "Eliminar" }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText("La categoría todavía tiene materiales.");
  await page.getByRole("dialog").getByRole("button", { name: "Cancelar" }).click();

  await page.goto("/admin/ubicaciones");
  await page.getByRole("button", { name: "Nueva" }).click();
  const location = page.getByRole("dialog", { name: "Nueva ubicación" });
  await location.getByLabel(/Nombre/).fill("Caja de tornillos");
  await choose(page, /Tipo/, "Contenedor");
  await choose(page, /Dentro de/, /Bodega E2E › Estante A/);
  await location.getByRole("button", { name: "Crear ubicación" }).click();
  await expect(page.getByRole("list", { name: "Ubicaciones" })).toContainText("Caja de tornillos");

  await page.getByRole("button", { name: "Eliminar Bodega E2E" }).click();
  await page.getByRole("dialog", { name: "Eliminar ubicación" }).getByRole("button", { name: "Eliminar" }).click();
  await expect(page.getByRole("dialog").getByRole("alert")).toContainText("La ubicación todavía tiene materiales o sububicaciones.");
});

test("any user can change their own password", async ({ page }) => {
  await login(page, "admin");
  await page.goto("/cuenta");
  const form = page.getByRole("form", { name: "Cambiar contraseña" });
  await form.getByLabel(/Contraseña actual/).fill("equivocada-12345");
  await form.getByLabel(/^Nueva contraseña/).fill("otra-clave-segura-1");
  await form.getByLabel(/Repite/).fill("otra-clave-segura-1");
  await form.getByRole("button", { name: "Cambiar contraseña" }).click();
  await expect(form.getByText("La contraseña actual no es correcta.")).toBeVisible();
});
