import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { consumable, item } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

function serveList(listCalls: { count: number }) {
  server.use(
    http.get(api("/items"), () => {
      listCalls.count += 1;
      return HttpResponse.json({ data: [item()], total: 1, limit: 25, offset: 0 });
    }),
  );
}

async function openCreate(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: /Nuevo/ }));
  return screen.findByRole("dialog", { name: "Nuevo material" });
}

describe("create item", () => {
  it("is not offered to viewers", async () => {
    useAuthenticatedApi("viewer");
    serveList({ count: 0 });
    renderApp("/materiales");
    await screen.findByText("Taladro inalámbrico");
    expect(screen.queryByRole("button", { name: /Nuevo/ })).not.toBeInTheDocument();
  });

  it("creates an asset, refreshes the list and opens the new item", async () => {
    useAuthenticatedApi("operator");
    const calls = { count: 0 };
    serveList(calls);
    let body: Record<string, unknown> = {};
    const created = item({ id: "44444444-4444-4444-4444-444444444444", code: "ISK-HER-010", description: "Nivel láser" });
    server.use(
      http.post(api("/items"), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(created, { status: 201 });
      }),
      http.get(api(`/items/${created.id}`), () => HttpResponse.json(created)),
    );
    const user = userEvent.setup();
    renderApp("/materiales");
    const dialog = await openCreate(user);

    expect(within(dialog).queryByLabelText("Código personalizado")).not.toBeInTheDocument();
    await user.type(within(dialog).getByLabelText(/Descripción/), "Nivel láser");
    await user.click(within(dialog).getByRole("combobox", { name: /Categoría/ }));
    await user.click(await screen.findByRole("option", { name: "Herramientas" }));
    await user.type(within(dialog).getByLabelText(/Mantenimiento cada/), "180");
    await user.click(within(dialog).getByRole("button", { name: "Añadir atributo" }));
    await user.type(within(dialog).getByLabelText("Nombre del atributo 1"), "marca");
    await user.type(within(dialog).getByLabelText("Valor del atributo 1"), "Bosch");
    const listCallsBefore = calls.count;
    await user.click(within(dialog).getByRole("button", { name: "Crear material" }));

    expect(await screen.findByRole("heading", { name: "ISK-HER-010" })).toBeInTheDocument();
    expect(body).toEqual({
      kind: "asset",
      description: "Nivel láser",
      category_id: 1,
      notes: "",
      maintenance_interval_days: 180,
      attributes: { marca: "Bosch" },
    });
    expect(listCallsBefore).toBeGreaterThan(0);
    expect(await screen.findByText("Material ISK-HER-010 creado.")).toBeInTheDocument();
  });

  it("creates a consumable with unit and opening stock", async () => {
    useAuthenticatedApi("operator");
    serveList({ count: 0 });
    let body: Record<string, unknown> = {};
    const created = consumable({ code: "ISK-CON-009" });
    server.use(
      http.post(api("/items"), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(created, { status: 201 });
      }),
      http.get(api(`/items/${created.id}`), () => HttpResponse.json(created)),
    );
    const user = userEvent.setup();
    renderApp("/materiales");
    const dialog = await openCreate(user);

    await user.click(within(dialog).getByRole("radio", { name: "Consumible" }));
    await user.type(within(dialog).getByLabelText(/Descripción/), "Guantes de nitrilo");
    await user.click(within(dialog).getByRole("combobox", { name: /Categoría/ }));
    await user.click(await screen.findByRole("option", { name: "Consumibles" }));
    await user.type(within(dialog).getByLabelText(/Unidad/), "caja");
    await user.clear(within(dialog).getByLabelText("Existencias iniciales"));
    await user.type(within(dialog).getByLabelText("Existencias iniciales"), "12");
    await user.clear(within(dialog).getByLabelText("Stock mínimo"));
    await user.type(within(dialog).getByLabelText("Stock mínimo"), "4");
    await user.click(within(dialog).getByRole("button", { name: "Crear material" }));

    await screen.findByRole("heading", { name: "ISK-CON-009" });
    expect(body).toMatchObject({ kind: "consumable", unit: "caja", stock_current: 12, stock_min: 4 });
    expect(body).not.toHaveProperty("maintenance_interval_days");
  });

  it("validates required fields before calling the API", async () => {
    useAuthenticatedApi("operator");
    serveList({ count: 0 });
    const user = userEvent.setup();
    renderApp("/materiales");
    const dialog = await openCreate(user);
    await user.click(within(dialog).getByRole("radio", { name: "Consumible" }));
    await user.click(within(dialog).getByRole("button", { name: "Crear material" }));
    expect(await within(dialog).findByText("Describe el material.")).toBeInTheDocument();
    expect(within(dialog).getByText("Elige una categoría.")).toBeInTheDocument();
    expect(within(dialog).getByText(/Indica la unidad/)).toBeInTheDocument();
  });

  it("maps 422 field errors from the server onto the form", async () => {
    useAuthenticatedApi("operator");
    serveList({ count: 0 });
    server.use(
      http.post(api("/items"), () =>
        HttpResponse.json(
          errorBody("validation_failed", "invalid", { description: "ya existe un material igual", location_id: "ubicación inválida" }),
          { status: 422 },
        ),
      ),
    );
    const user = userEvent.setup();
    renderApp("/materiales");
    const dialog = await openCreate(user);
    await user.type(within(dialog).getByLabelText(/Descripción/), "Taladro");
    await user.click(within(dialog).getByRole("combobox", { name: /Categoría/ }));
    await user.click(await screen.findByRole("option", { name: "Herramientas" }));
    await user.click(within(dialog).getByRole("button", { name: "Crear material" }));

    expect(await within(dialog).findByText("ya existe un material igual")).toBeInTheDocument();
    expect(within(dialog).getByText("ubicación inválida")).toBeInTheDocument();
    expect(within(dialog).getByLabelText(/Descripción/)).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("dialog", { name: "Nuevo material" })).toBeInTheDocument();
  });

  it("lets only admins type a custom code", async () => {
    useAuthenticatedApi("admin");
    serveList({ count: 0 });
    let body: Record<string, unknown> = {};
    const created = item({ code: "ISK-HER-500" });
    server.use(
      http.post(api("/items"), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(created, { status: 201 });
      }),
      http.get(api(`/items/${created.id}`), () => HttpResponse.json(created)),
    );
    const user = userEvent.setup();
    renderApp("/materiales");
    const dialog = await openCreate(user);
    await user.type(within(dialog).getByLabelText("Código personalizado"), "isk-her-500");
    await user.type(within(dialog).getByLabelText(/Descripción/), "Taladro");
    await user.click(within(dialog).getByRole("combobox", { name: /Categoría/ }));
    await user.click(await screen.findByRole("option", { name: "Herramientas" }));
    await user.click(within(dialog).getByRole("button", { name: "Crear material" }));
    await waitFor(() => expect(body.code).toBe("ISK-HER-500"));
  });
});
