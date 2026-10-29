import { expect, test } from "@playwright/test";
import { choose, login } from "./helpers";

test("create a supplier, order 3 new assets, mark ordered, receive and find 3 distinct items", async ({ page }) => {
  await login(page, "operator");

  await page.goto("/proveedores");
  await page.getByRole("button", { name: "Nuevo" }).click();
  const supplierSheet = page.getByRole("dialog", { name: "Nuevo proveedor" });
  await supplierSheet.getByLabel("Nombre").fill("Proveedor E2E");
  await supplierSheet.getByRole("button", { name: "Crear proveedor" }).click();
  await expect(page.getByText("Proveedor Proveedor E2E creado.", { exact: true })).toBeVisible();

  await page.goto("/ordenes-compra");
  await page.getByRole("button", { name: "Nueva" }).click();
  const create = page.getByRole("dialog", { name: "Nueva orden de compra" });
  await choose(page, /Proveedor/, "Proveedor E2E");
  await create.getByRole("button", { name: "Añadir línea" }).click();
  const line = create.getByRole("region", { name: "Línea 1" });
  await line.getByText("Ítem nuevo", { exact: true }).click();
  await line.getByLabel("Descripción").fill("Laptop compra E2E");
  await choose(page, /Categoría/, "Herramientas");
  await line.getByText("Activo", { exact: true }).click();
  await line.getByLabel(/^Cantidad/).fill("3");
  await expect(line.getByText("Se crearán 3 ítems con códigos distintos.")).toBeVisible();
  await line.getByLabel(/Costo unitario/).fill("450.50");
  await expect(create.getByText(/Total US\$ 1,351\.50/)).toBeVisible();
  await create.getByRole("button", { name: "Crear orden" }).click();
  await expect(page.getByText(/Orden #\d+ creada como borrador\./).first()).toBeVisible();

  await page.getByRole("link", { name: /Proveedor E2E/ }).first().click();
  await page.getByRole("button", { name: "Marcar como pedida" }).click();
  await expect(page.getByText(/Orden #\d+ marcada como pedida\./).first()).toBeVisible();

  await page.getByRole("button", { name: "Recibir mercancía" }).click();
  const receive = page.getByRole("dialog", { name: "Recibir mercancía" });
  await choose(page, /Ubicación de destino/, /^Bodega E2E$/);
  await receive.getByRole("button", { name: "Registrar recepción" }).click();
  await expect(page.getByText(/Recepción registrada en la orden #\d+\./).first()).toBeVisible();

  await page.goto("/materiales");
  await page.getByLabel("Buscar materiales").fill("Laptop compra E2E");
  await expect(page.getByRole("link", { name: /Laptop compra E2E/ })).toHaveCount(3);
});

test("a viewer reads suppliers and orders but sees no write actions", async ({ page }) => {
  await login(page, "viewer");
  await page.goto("/proveedores");
  await expect(page.getByRole("heading", { name: "Proveedores" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Nuevo" })).toHaveCount(0);
  await page.goto("/ordenes-compra");
  await expect(page.getByRole("heading", { name: "Órdenes de compra" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Nueva" })).toHaveCount(0);
});

test("the mobile bar offers purchase orders, and a supplier can be added from the order form", async ({ page }) => {
  await login(page, "operator");
  const bar = page.getByRole("navigation", { name: "Navegación principal" });
  await expect(bar.getByRole("link", { name: "Órdenes de compra" })).toBeVisible();
  await expect(bar.getByRole("link", { name: "Proveedores" })).toHaveCount(0);

  await bar.getByRole("link", { name: "Órdenes de compra" }).click();
  await page.getByRole("button", { name: "Nueva" }).click();
  const order = page.getByRole("dialog", { name: "Nueva orden de compra" });
  await order.getByRole("button", { name: "Nuevo proveedor" }).click();
  const supplier = page.getByRole("dialog", { name: "Nuevo proveedor" });
  await supplier.getByLabel("Nombre").fill("Proveedor Inline E2E");
  await supplier.getByRole("button", { name: "Crear proveedor" }).click();
  await expect(page.getByText("Proveedor Proveedor Inline E2E creado.").first()).toBeVisible();
  await expect(order.getByRole("combobox", { name: /Proveedor/ })).toContainText("Proveedor Inline E2E");
});
