import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { consumable, page, purchaseOrder, supplier } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const cinta = consumable({ id: "22222222-2222-2222-2222-222222222222", code: "ISK-CON-001", description: "Cinta aislante", unit: "rollo" });

function serveCommon(orders = [purchaseOrder()]) {
  server.use(
    http.get(api("/suppliers"), () => HttpResponse.json(page([supplier(), supplier({ id: 2, name: "Pinturas Norte" })]))),
    http.get(api("/purchase-orders"), () => HttpResponse.json(page(orders))),
    http.get(api("/items"), () => HttpResponse.json(page([cinta]))),
  );
}

async function openCreate(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: /Nueva/ }));
  return screen.findByRole("dialog", { name: "Nueva orden de compra" });
}

async function pickSupplier(user: ReturnType<typeof userEvent.setup>, dialog: HTMLElement, name = "Ferretería Sur") {
  await user.click(within(dialog).getByRole("combobox", { name: /Proveedor/ }));
  await user.click(await screen.findByRole("option", { name }));
}

afterEach(() => vi.restoreAllMocks());

describe("purchase orders list", () => {
  it("lists orders with status and total, and keeps the filters in the URL", async () => {
    useAuthenticatedApi("operator");
    const seen: URLSearchParams[] = [];
    server.use(
      http.get(api("/suppliers"), () => HttpResponse.json(page([supplier()]))),
      http.get(api("/purchase-orders"), ({ request }) => {
        seen.push(new URL(request.url).searchParams);
        return HttpResponse.json(page([purchaseOrder()]));
      }),
    );
    renderApp("/ordenes-compra?status=ordered&supplier_id=1");
    const list = await screen.findByRole("list", { name: "Órdenes de compra" });
    expect(within(list).getByText("#7 · Ferretería Sur")).toBeInTheDocument();
    expect(within(list).getByText("Pedida")).toBeInTheDocument();
    expect(within(list).getByText(/US\$ 505\.00/)).toBeInTheDocument();
    expect(seen[0].get("status")).toBe("ordered");
    expect(seen[0].get("supplier_id")).toBe("1");
  });

  it("hides the create button from viewers", async () => {
    useAuthenticatedApi("viewer");
    serveCommon();
    renderApp("/ordenes-compra");
    await screen.findByText("#7 · Ferretería Sur");
    expect(screen.queryByRole("button", { name: /Nueva/ })).not.toBeInTheDocument();
  });
});

describe("create purchase order", () => {
  it("saves a draft with an existing consumable line in minor units", async () => {
    useAuthenticatedApi("operator");
    serveCommon();
    let body: unknown;
    server.use(
      http.post(api("/purchase-orders"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(purchaseOrder({ id: 8, status: "draft" }), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra");
    const dialog = await openCreate(user);
    await pickSupplier(user, dialog);
    await user.click(within(dialog).getByRole("button", { name: "Añadir línea" }));
    const line = within(dialog).getByRole("region", { name: "Línea 1" });
    await user.type(within(line).getByLabelText("Buscar consumible"), "cinta");
    await user.click(await within(line).findByRole("button", { name: "Elegir ISK-CON-001" }));
    expect(within(line).getByText("ISK-CON-001 · Cinta aislante")).toBeInTheDocument();
    await user.clear(within(line).getByLabelText(/Cantidad/));
    await user.type(within(line).getByLabelText(/Cantidad/), "5");
    await user.type(within(line).getByLabelText(/Costo unitario/), "2.5");
    expect(within(dialog).getByText("Total US$ 12.50")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Crear orden" }));

    expect(await screen.findByText("Orden #8 creada como borrador.")).toBeInTheDocument();
    expect(body).toEqual({
      supplier_id: 1,
      currency: "USD",
      exchange_rate: null,
      expected_at: null,
      notes: "",
      lines: [{ item_id: cinta.id, quantity: 5, unit_cost_minor: 250 }],
    });
  });

  it("defines a new asset inline, hints how many items it creates and bills in bolivares", async () => {
    useAuthenticatedApi("operator");
    serveCommon();
    let body: unknown;
    server.use(
      http.post(api("/purchase-orders"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(purchaseOrder({ id: 9, status: "draft", currency: "VES" }), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra");
    const dialog = await openCreate(user);
    await pickSupplier(user, dialog);
    await user.click(within(dialog).getByLabelText("Bolívares (Bs.)"));
    await user.type(within(dialog).getByLabelText("Tasa de cambio"), "36,5");
    await user.click(within(dialog).getByRole("button", { name: "Añadir línea" }));
    const line = within(dialog).getByRole("region", { name: "Línea 1" });
    await user.click(within(line).getByLabelText("Ítem nuevo"));
    await user.type(within(line).getByLabelText(/Descripción/), "Taladro percutor");
    await user.click(within(line).getByRole("combobox", { name: /Categoría/ }));
    await user.click(await screen.findByRole("option", { name: "Herramientas" }));
    await user.click(within(line).getByLabelText("Activo"));
    await user.clear(within(line).getByLabelText(/Cantidad/));
    await user.type(within(line).getByLabelText(/Cantidad/), "3");
    expect(within(line).getByText("Se crearán 3 ítems con códigos distintos.")).toBeInTheDocument();
    await user.type(within(line).getByLabelText(/Costo unitario \(Bs\.\)/), "120.50");
    expect(within(dialog).getByText("Total Bs. 361.50")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Crear orden" }));

    expect(await screen.findByText("Orden #9 creada como borrador.")).toBeInTheDocument();
    expect(body).toEqual({
      supplier_id: 1,
      currency: "VES",
      exchange_rate: 36.5,
      expected_at: null,
      notes: "",
      lines: [{ new_item: { description: "Taladro percutor", category_id: 1, kind: "asset" }, quantity: 3, unit_cost_minor: 12050 }],
    });
  });

  it("rejects more than 100 units of a new asset before calling the API", async () => {
    useAuthenticatedApi("operator");
    serveCommon();
    const user = userEvent.setup();
    renderApp("/ordenes-compra");
    const dialog = await openCreate(user);
    await pickSupplier(user, dialog);
    await user.click(within(dialog).getByRole("button", { name: "Añadir línea" }));
    const line = within(dialog).getByRole("region", { name: "Línea 1" });
    await user.click(within(line).getByLabelText("Ítem nuevo"));
    await user.type(within(line).getByLabelText(/Descripción/), "Silla");
    await user.click(within(line).getByRole("combobox", { name: /Categoría/ }));
    await user.click(await screen.findByRole("option", { name: "Herramientas" }));
    await user.click(within(line).getByLabelText("Activo"));
    await user.clear(within(line).getByLabelText(/Cantidad/));
    await user.type(within(line).getByLabelText(/Cantidad/), "101");
    await user.type(within(line).getByLabelText(/Costo unitario/), "10");
    await user.click(within(dialog).getByRole("button", { name: "Crear orden" }));
    expect(await within(line).findByText("Máximo 100 ítems por línea de activos.")).toBeInTheDocument();
  });

  it("lands a 422 on the new-item description input", async () => {
    useAuthenticatedApi("operator");
    serveCommon();
    server.use(
      http.post(api("/purchase-orders"), () =>
        HttpResponse.json(errorBody("validation_failed", "invalid", { "lines[0].new_item.description": "Ya existe un ítem con esa descripción." }), { status: 422 }),
      ),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra");
    const dialog = await openCreate(user);
    await pickSupplier(user, dialog);
    await user.click(within(dialog).getByRole("button", { name: "Añadir línea" }));
    const line = within(dialog).getByRole("region", { name: "Línea 1" });
    await user.click(within(line).getByLabelText("Ítem nuevo"));
    await user.type(within(line).getByLabelText(/Descripción/), "Cinta aislante");
    await user.click(within(line).getByRole("combobox", { name: /Categoría/ }));
    await user.click(await screen.findByRole("option", { name: "Consumibles" }));
    await user.type(within(line).getByLabelText(/Unidad/), "rollo");
    await user.type(within(line).getByLabelText(/Costo unitario/), "2");
    await user.click(within(dialog).getByRole("button", { name: "Crear orden" }));
    await waitFor(() => expect(within(line).getByLabelText(/Descripción/)).toHaveAccessibleDescription("Ya existe un ítem con esa descripción."));
  });
});

describe("purchase order detail", () => {
  it("offers receive and cancel to operators and nothing to viewers", async () => {
    server.use(http.get(api("/purchase-orders/7"), () => HttpResponse.json(purchaseOrder())));
    useAuthenticatedApi("viewer");
    renderApp("/ordenes-compra/7");
    expect(await screen.findByRole("heading", { name: "Orden #7" })).toBeInTheDocument();
    expect(screen.getByText("Total")).toBeInTheDocument();
    expect(screen.getByText("Taladro percutor")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Recibir|Cancelar|Editar|Marcar/ })).not.toBeInTheDocument();
  });

  it("marks a draft as ordered", async () => {
    useAuthenticatedApi("operator");
    let current = purchaseOrder({ status: "draft", ordered_at: null });
    server.use(
      http.get(api("/purchase-orders/7"), () => HttpResponse.json(current)),
      http.post(api("/purchase-orders/7/order"), () => {
        current = purchaseOrder();
        return HttpResponse.json(current);
      }),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra/7");
    await user.click(await screen.findByRole("button", { name: "Marcar como pedida" }));
    expect(await screen.findByText("Orden #7 marcada como pedida.")).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Recibir mercancía" })).toBeInTheDocument();
  });

  it("cancels an order after confirming and shows a conflict in place", async () => {
    useAuthenticatedApi("operator");
    server.use(
      http.get(api("/purchase-orders/7"), () => HttpResponse.json(purchaseOrder())),
      http.post(api("/purchase-orders/7/cancel"), () => HttpResponse.json(errorBody("invalid_state", "x"), { status: 409 })),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra/7");
    await user.click(await screen.findByRole("button", { name: "Cancelar orden" }));
    const dialog = await screen.findByRole("dialog", { name: "Cancelar orden" });
    await user.click(within(dialog).getByRole("button", { name: "Cancelar orden" }));
    expect(await within(dialog).findByText(/ya no está en un estado que permita esta acción/)).toBeInTheDocument();
  });
});

describe("receive purchase order", () => {
  async function openReceive(user: ReturnType<typeof userEvent.setup>) {
    await user.click(await screen.findByRole("button", { name: "Recibir mercancía" }));
    return screen.findByRole("dialog", { name: "Recibir mercancía" });
  }

  it("posts the received lines, location and an idempotency key, then confirms", async () => {
    useAuthenticatedApi("operator");
    vi.spyOn(globalThis.crypto, "randomUUID").mockReturnValue("00000000-0000-4000-8000-000000000001");
    let body: unknown;
    const after = purchaseOrder({ status: "partially_received" });
    server.use(
      http.get(api("/purchase-orders/7"), () => HttpResponse.json(purchaseOrder())),
      http.post(api("/purchase-orders/7/receive"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(
          {
            receipt: { id: 1, order_id: 7, location_id: 10, received_by: null, received_at: "2026-10-05T00:00:00Z", note: "Guía 55", lines: [{ line_id: 100, quantity: 6, items: [{ item_id: cinta.id, code: "ISK-CON-001", quantity: 6 }] }] },
            order: after,
          },
          { status: 201 },
        );
      }),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra/7");
    const dialog = await openReceive(user);
    // The second line (a new asset) still has 3 of 4 pending; the form starts with everything pending.
    expect(within(dialog).getByLabelText("Cantidad de Cinta aislante")).toHaveValue("10");
    expect(within(dialog).getByLabelText("Cantidad de Taladro percutor")).toHaveValue("3");
    await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de destino/ }));
    await user.click(await screen.findByRole("option", { name: "Bodega › Estante A" }));
    await user.clear(within(dialog).getByLabelText("Cantidad de Cinta aislante"));
    await user.type(within(dialog).getByLabelText("Cantidad de Cinta aislante"), "6");
    await user.clear(within(dialog).getByLabelText("Cantidad de Taladro percutor"));
    await user.type(within(dialog).getByLabelText(/^Nota/), "Guía 55");
    await user.click(within(dialog).getByRole("button", { name: "Registrar recepción" }));

    expect(await screen.findByText("Recepción registrada en la orden #7.")).toBeInTheDocument();
    expect(body).toEqual({
      location_id: 11,
      note: "Guía 55",
      client_key: "00000000-0000-4000-8000-000000000001",
      lines: [{ line_id: 100, quantity: 6 }],
    });
  });

  it("does not let you receive more than what is pending", async () => {
    useAuthenticatedApi("operator");
    server.use(http.get(api("/purchase-orders/7"), () => HttpResponse.json(purchaseOrder())));
    const user = userEvent.setup();
    renderApp("/ordenes-compra/7");
    const dialog = await openReceive(user);
    await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de destino/ }));
    await user.click(await screen.findByRole("option", { name: "Bodega" }));
    await user.clear(within(dialog).getByLabelText("Cantidad de Taladro percutor"));
    await user.type(within(dialog).getByLabelText("Cantidad de Taladro percutor"), "4");
    await user.click(within(dialog).getByRole("button", { name: "Registrar recepción" }));
    expect(await within(dialog).findByText("Máximo 3: es lo que falta por recibir.")).toBeInTheDocument();
  });

  it("lands a server over-receipt on the right quantity input", async () => {
    useAuthenticatedApi("operator");
    server.use(
      http.get(api("/purchase-orders/7"), () => HttpResponse.json(purchaseOrder())),
      http.post(api("/purchase-orders/7/receive"), () =>
        HttpResponse.json(errorBody("over_receipt", "too many", { "lines[0].quantity": "No puedes recibir más de lo pendiente." }), { status: 422 }),
      ),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra/7");
    const dialog = await openReceive(user);
    await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de destino/ }));
    await user.click(await screen.findByRole("option", { name: "Bodega" }));
    // Skip the first line: the request carries only the asset line, which is index 0 on the server.
    await user.clear(within(dialog).getByLabelText("Cantidad de Cinta aislante"));
    await user.click(within(dialog).getByRole("button", { name: "Registrar recepción" }));
    await waitFor(() =>
      expect(within(dialog).getByLabelText("Cantidad de Taladro percutor")).toHaveAccessibleDescription("No puedes recibir más de lo pendiente."),
    );
    expect(within(dialog).getByLabelText("Cantidad de Cinta aislante")).not.toHaveAttribute("aria-invalid");
  });
});

describe("adding a supplier from the order form", () => {
  it("creates the supplier in a nested sheet and selects it in the order", async () => {
    useAuthenticatedApi("operator");
    const suppliers = [supplier()];
    let created: unknown;
    server.use(
      http.get(api("/suppliers"), () => HttpResponse.json(page(suppliers))),
      http.get(api("/purchase-orders"), () => HttpResponse.json(page([]))),
      http.get(api("/items"), () => HttpResponse.json(page([cinta]))),
      http.post(api("/suppliers"), async ({ request }) => {
        created = await request.json();
        const saved = supplier({ id: 9, name: "Eléctrica Centro" });
        suppliers.push(saved);
        return HttpResponse.json(saved, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/ordenes-compra");
    const order = await openCreate(user);
    await user.click(within(order).getByRole("button", { name: "Nuevo proveedor" }));
    const sheet = await screen.findByRole("dialog", { name: "Nuevo proveedor" });
    await user.type(within(sheet).getByLabelText(/Nombre/), "Eléctrica Centro");
    await user.click(within(sheet).getByRole("button", { name: "Crear proveedor" }));
    expect(await screen.findByText("Proveedor Eléctrica Centro creado.")).toBeInTheDocument();
    expect(created).toMatchObject({ name: "Eléctrica Centro" });
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Nuevo proveedor" })).not.toBeInTheDocument());
    await waitFor(() => expect(within(order).getByRole("combobox", { name: /Proveedor/ })).toHaveTextContent("Eléctrica Centro"));
  });

  it("does not offer it while editing an existing draft", async () => {
    useAuthenticatedApi("operator");
    const draft = purchaseOrder({ status: "draft" });
    serveCommon([draft]);
    server.use(http.get(api("/purchase-orders/:id"), () => HttpResponse.json(draft)));
    const user = userEvent.setup();
    renderApp("/ordenes-compra/1");
    await user.click(await screen.findByRole("button", { name: /Editar/ }));
    const dialog = await screen.findByRole("dialog", { name: "Editar orden" });
    expect(within(dialog).queryByRole("button", { name: "Nuevo proveedor" })).not.toBeInTheDocument();
  });
});
