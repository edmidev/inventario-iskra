import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import type { Item } from "../lib/api/schemas";
import { consumable, item, onLoanItem, stock } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const serveItem = (value: Item) => server.use(http.get(api(`/items/${value.id}`), () => HttpResponse.json(value)));

describe("item detail", () => {
  it("shows custody information for an overdue loan", async () => {
    useAuthenticatedApi("viewer");
    const loaned = onLoanItem();
    serveItem(loaned);
    renderApp(`/materiales/${loaned.id}`);

    expect(await screen.findByRole("heading", { name: "ISK-HER-002" })).toBeInTheDocument();
    const custody = screen.getByRole("region", { name: "Préstamo vigente" });
    expect(within(custody).getByRole("heading", { name: "Prestado a Juan Gómez" })).toBeInTheDocument();
    expect(custody).toHaveTextContent("Vencido");
    expect(screen.getAllByText("Prestado a Juan Gómez").length).toBeGreaterThan(0);
  });

  it("shows info grid with category, location, maintenance and attributes", async () => {
    useAuthenticatedApi("viewer");
    const asset = item({ maintenance_overdue: true, notes: "Revisar batería" });
    serveItem(asset);
    renderApp(`/materiales/${asset.id}`);

    expect(await screen.findByText("Taladro inalámbrico")).toBeInTheDocument();
    expect(screen.getByText("Herramientas")).toBeInTheDocument();
    expect(await screen.findByText("Bodega › Estante A")).toBeInTheDocument();
    expect(screen.getByText("90 días")).toBeInTheDocument();
    expect(screen.getByText("Mantenimiento vencido")).toBeInTheDocument();
    expect(screen.getByText("marca")).toBeInTheDocument();
    expect(screen.getByText("Makita")).toBeInTheDocument();
    expect(screen.getByText("Revisar batería")).toBeInTheDocument();
  });

  it("shows the stock per location, the total and a low-stock warning for consumables", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable({ stocks: [stock(10, 2), stock(11, 1)], stock_min: 5 });
    serveItem(supply);
    renderApp(`/materiales/${supply.id}`);

    const section = await screen.findByRole("region", { name: "Stock por ubicación" });
    const rows = within(within(section).getByRole("list", { name: "Existencias por ubicación" })).getAllByRole("listitem");
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent("Bodega2 caja");
    expect(rows[1]).toHaveTextContent("Estante A1 caja");
    expect(within(section).getByText("Total")).toBeInTheDocument();
    expect(within(section).getByText("3 caja")).toBeInTheDocument();
    expect(within(section).getByText("En o por debajo del mínimo de 5 caja.")).toBeInTheDocument();
    expect(screen.getAllByText("Stock bajo").length).toBeGreaterThan(0);
  });

  it("does not show a single location row for consumables, nor the section for assets", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable({ stocks: [stock(10, 9)], stock_min: 2, low_stock: false });
    serveItem(supply);
    renderApp(`/materiales/${supply.id}`);
    await screen.findByRole("region", { name: "Stock por ubicación" });
    expect(screen.queryByText("Ubicación")).not.toBeInTheDocument();
    expect(screen.queryByText(/por debajo del mínimo/)).not.toBeInTheDocument();
  });

  it("says so when a consumable has no stock anywhere", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable({ stock_current: 0 });
    serveItem(supply);
    renderApp(`/materiales/${supply.id}`);
    expect(await screen.findByText("No hay existencias en ninguna ubicación.")).toBeInTheDocument();
  });

  it("keeps the location row and no stock section for assets", async () => {
    useAuthenticatedApi("viewer");
    const asset = item();
    serveItem(asset);
    renderApp(`/materiales/${asset.id}`);
    await screen.findByText("Ubicación");
    expect(screen.queryByRole("region", { name: "Stock por ubicación" })).not.toBeInTheDocument();
  });

  it("copies the code to the clipboard", async () => {
    useAuthenticatedApi("viewer");
    const asset = item();
    serveItem(asset);
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Copiar código" }));
    expect(await screen.findByText("Código ISK-HER-001 copiado.")).toBeInTheDocument();
    expect(await navigator.clipboard.readText()).toBe("ISK-HER-001");
  });

  it("reports a missing item", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api("/items/nope"), () => HttpResponse.json(errorBody("not_found", "x"), { status: 404 })));
    renderApp("/materiales/nope");
    expect(await screen.findByRole("alert")).toHaveTextContent("no existe o fue eliminado");
  });
});

describe("item detail permissions", () => {
  it("hides edit and delete from viewers", async () => {
    useAuthenticatedApi("viewer");
    const asset = item();
    serveItem(asset);
    renderApp(`/materiales/${asset.id}`);
    await screen.findByRole("heading", { name: "ISK-HER-001" });
    expect(screen.queryByRole("button", { name: /editar/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /eliminar/i })).not.toBeInTheDocument();
  });

  it("shows edit and delete to operators", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serveItem(asset);
    renderApp(`/materiales/${asset.id}`);
    expect(await screen.findByRole("button", { name: /editar/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /eliminar/i })).toBeInTheDocument();
  });
});

describe("item editing", () => {
  it("sends only what changed and refreshes the detail", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    let body: Record<string, unknown> = {};
    server.use(
      http.get(api(`/items/${asset.id}`), () => HttpResponse.json(asset)),
      http.patch(api(`/items/${asset.id}`), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ ...asset, description: "Taladro percutor" });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: /editar/i }));
    const dialog = await screen.findByRole("dialog", { name: "Editar material" });
    const description = within(dialog).getByLabelText(/Descripción/);
    await user.clear(description);
    await user.type(description, "Taladro percutor");
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));

    expect(await screen.findByRole("heading", { name: "ISK-HER-001" })).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(screen.getByText("Taladro percutor")).toBeInTheDocument();
    expect(body).toMatchObject({ description: "Taladro percutor", category_id: 1, maintenance_interval_days: 90 });
    expect(body).not.toHaveProperty("location_id");
    expect(body).not.toHaveProperty("attributes");
    expect(body).not.toHaveProperty("code");
    expect(body).not.toHaveProperty("kind");
  });

  it("edits a consumable without a location field and never sends location or stock", async () => {
    useAuthenticatedApi("operator");
    const supply = consumable({ stocks: [stock(10, 2), stock(11, 1)] });
    let body: Record<string, unknown> = {};
    server.use(
      http.get(api(`/items/${supply.id}`), () => HttpResponse.json(supply)),
      http.patch(api(`/items/${supply.id}`), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({ ...supply, stock_min: 8 });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${supply.id}`);
    await user.click(await screen.findByRole("button", { name: /editar/i }));
    const dialog = await screen.findByRole("dialog", { name: "Editar material" });
    expect(within(dialog).queryByRole("combobox", { name: /^Ubicación/ })).not.toBeInTheDocument();
    expect(within(dialog).queryByText("Stock inicial por ubicación")).not.toBeInTheDocument();
    expect(within(dialog).getByText(/se cambian con entradas, salidas, ajustes y traslados/)).toBeInTheDocument();
    await user.clear(within(dialog).getByLabelText("Stock mínimo"));
    await user.type(within(dialog).getByLabelText("Stock mínimo"), "8");
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(body).toMatchObject({ unit: "caja", stock_min: 8 });
    for (const key of ["location_id", "stocks", "stock_current"]) expect(body).not.toHaveProperty(key);
  });

  it("shows the location select when editing an asset", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serveItem(asset);
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: /editar/i }));
    const dialog = await screen.findByRole("dialog", { name: "Editar material" });
    expect(within(dialog).getByRole("combobox", { name: /^Ubicación/ })).toBeInTheDocument();
  });

  it("maps 422 field errors onto the inputs", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    server.use(
      http.get(api(`/items/${asset.id}`), () => HttpResponse.json(asset)),
      http.patch(api(`/items/${asset.id}`), () =>
        HttpResponse.json(errorBody("validation_failed", "invalid", { notes: "Demasiado largo en el servidor" }), { status: 422 }),
      ),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: /editar/i }));
    const dialog = await screen.findByRole("dialog", { name: "Editar material" });
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await within(dialog).findByText("Demasiado largo en el servidor")).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Notas")).toHaveAttribute("aria-invalid", "true");
  });

  it("explains a conflict while the item is under repair", async () => {
    useAuthenticatedApi("operator");
    const asset = item({ status: "in_repair", availability: "unavailable" });
    server.use(
      http.get(api(`/items/${asset.id}`), () => HttpResponse.json(asset)),
      http.patch(api(`/items/${asset.id}`), () => HttpResponse.json(errorBody("item_under_repair", "x"), { status: 409 })),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: /editar/i }));
    const dialog = await screen.findByRole("dialog", { name: "Editar material" });
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("falla abierta");
  });
});

describe("item deletion", () => {
  it("confirms, deletes and returns to the list", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    let deleted = false;
    server.use(
      http.get(api(`/items/${asset.id}`), () => HttpResponse.json(asset)),
      http.delete(api(`/items/${asset.id}`), () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
      http.get(api("/items"), () => HttpResponse.json({ data: [], total: 0, limit: 25, offset: 0 })),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: /eliminar/i }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar material" });
    expect(deleted).toBe(false);
    await user.click(within(dialog).getByRole("button", { name: "Eliminar" }));
    expect(await screen.findByRole("heading", { name: "Materiales" })).toBeInTheDocument();
    expect(deleted).toBe(true);
  });

  it("shows a readable message when the item is on loan", async () => {
    useAuthenticatedApi("operator");
    const loaned = onLoanItem();
    server.use(
      http.get(api(`/items/${loaned.id}`), () => HttpResponse.json(loaned)),
      http.delete(api(`/items/${loaned.id}`), () => HttpResponse.json(errorBody("item_on_loan", "x"), { status: 409 })),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${loaned.id}`);
    await user.click(await screen.findByRole("button", { name: /eliminar/i }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar material" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("El material está prestado");
  });
});
