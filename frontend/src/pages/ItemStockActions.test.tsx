import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { dashboardKey } from "../features/dashboard/api/useDashboardSummary";
import type { Item } from "../lib/api/schemas";
import { consumable, item, movement, onLoanItem, summary } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

function serve(value: Item) {
  const state = { current: value, gets: 0 };
  server.use(http.get(api(`/items/${value.id}`), () => {
    state.gets += 1;
    return HttpResponse.json(state.current);
  }));
  return state;
}

const result = (stock: number, qty: number) => ({
  movement: movement({ qty, stock_after: stock }),
  stock_current: stock,
  location_id: 10,
});

describe("stock actions", () => {
  it("hides every action from viewers", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable();
    serve(supply);
    renderApp(`/materiales/${supply.id}`);
    await screen.findByRole("heading", { name: "ISK-CON-001" });
    expect(screen.queryByRole("region", { name: "Acciones" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Entrada" })).not.toBeInTheDocument();
  });

  it("registers an exit with a quick quantity in three taps, then refreshes item and dashboard", async () => {
    useAuthenticatedApi("operator");
    const supply = consumable({ stock_current: 12, stock_min: 5, low_stock: false });
    const state = serve(supply);
    let body: unknown;
    server.use(
      http.post(api(`/items/${supply.id}/movements`), async ({ request }) => {
        body = await request.json();
        state.current = { ...supply, stock_current: 7 };
        return HttpResponse.json(result(7, -5), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    const { client } = renderApp(`/materiales/${supply.id}`);
    await client.prefetchQuery({ queryKey: dashboardKey, queryFn: () => summary });

    await user.click(await screen.findByRole("button", { name: "Salida" }));
    const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
    await user.click(within(dialog).getByRole("button", { name: "5" }));
    expect(within(dialog).getByText(/Quedarán/)).toHaveTextContent("7 caja");
    await user.click(within(dialog).getByRole("button", { name: "Registrar salida" }));

    expect(await screen.findByText(/Salida registrada. Existencias: 7 caja/)).toBeInTheDocument();
    expect(body).toEqual({ type: "out", qty: 5 });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(state.gets).toBeGreaterThan(1));
    expect(client.getQueryState(dashboardKey)?.isInvalidated).toBe(true);
  });

  it("steps the quantity with the large buttons and warns when stock ends up low", async () => {
    useAuthenticatedApi("operator");
    const supply = consumable({ stock_current: 8, stock_min: 5, low_stock: false });
    serve(supply);
    const user = userEvent.setup();
    renderApp(`/materiales/${supply.id}`);
    await user.click(await screen.findByRole("button", { name: "Salida" }));
    const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
    await user.click(within(dialog).getByRole("button", { name: "Aumentar cantidad" }));
    await user.click(within(dialog).getByRole("button", { name: "Aumentar cantidad" }));
    expect(within(dialog).getByLabelText(/Cantidad que sale/)).toHaveValue(3);
    expect(within(dialog).getByText(/Quedarán/)).toHaveTextContent("5 caja");
    expect(within(dialog).getByText("Quedará en o por debajo del stock mínimo.")).toBeInTheDocument();
  });

  it("explains insufficient stock returned by the server", async () => {
    useAuthenticatedApi("operator");
    const supply = consumable({ stock_current: 3 });
    serve(supply);
    server.use(
      http.post(api(`/items/${supply.id}/movements`), () => HttpResponse.json(errorBody("insufficient_stock", "x"), { status: 409 })),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${supply.id}`);
    await user.click(await screen.findByRole("button", { name: "Salida" }));
    const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
    await user.click(within(dialog).getByRole("button", { name: "Registrar salida" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("No hay existencias suficientes");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("maps 422 field errors onto the quantity and note", async () => {
    useAuthenticatedApi("operator");
    const supply = consumable();
    serve(supply);
    server.use(
      http.post(api(`/items/${supply.id}/movements`), () =>
        HttpResponse.json(errorBody("validation_failed", "x", { note: "Nota inválida en el servidor" }), { status: 422 }),
      ),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${supply.id}`);
    await user.click(await screen.findByRole("button", { name: "Entrada" }));
    const dialog = await screen.findByRole("dialog", { name: "Entrada de existencias" });
    await user.click(within(dialog).getByRole("button", { name: "Registrar entrada" }));
    expect(await within(dialog).findByText("Nota inválida en el servidor")).toBeInTheDocument();
  });

  it("requires a note for an adjustment and sends the absolute count", async () => {
    useAuthenticatedApi("operator");
    const supply = consumable({ stock_current: 4 });
    serve(supply);
    let body: unknown;
    server.use(
      http.post(api(`/items/${supply.id}/movements`), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(result(9, 5), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${supply.id}`);
    await user.click(await screen.findByRole("button", { name: "Ajuste" }));
    const dialog = await screen.findByRole("dialog", { name: "Ajuste de inventario" });
    const qty = within(dialog).getByLabelText(/Cantidad contada/);
    expect(qty).toHaveValue(4);
    await user.clear(qty);
    await user.type(qty, "9");
    await user.click(within(dialog).getByRole("button", { name: "Guardar ajuste" }));
    expect(await within(dialog).findByText("Explica el motivo del ajuste.")).toBeInTheDocument();
    expect(body).toBeUndefined();
    await user.type(within(dialog).getByLabelText(/Motivo del ajuste/), "Conteo físico");
    await user.click(within(dialog).getByRole("button", { name: "Guardar ajuste" }));
    expect(await screen.findByText(/Ajuste registrado/)).toBeInTheDocument();
    expect(body).toEqual({ type: "adjust", qty: 9, note: "Conteo físico" });
  });

  it("transfers an asset to another location", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    let body: unknown;
    server.use(
      http.post(api(`/items/${asset.id}/movements`), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ movement: movement({ type: "transfer", qty: 0 }), stock_current: 1, location_id: 10 }, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Trasladar" }));
    const dialog = await screen.findByRole("dialog", { name: "Trasladar material" });
    await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de destino/ }));
    expect(screen.queryByRole("option", { name: "Bodega › Estante A" })).not.toBeInTheDocument();
    await user.click(await screen.findByRole("option", { name: "Bodega" }));
    await user.click(within(dialog).getByRole("button", { name: "Trasladar" }));
    expect(await screen.findByText("Material trasladado a Bodega.")).toBeInTheDocument();
    expect(body).toEqual({ type: "transfer", to_location_id: 10 });
  });

  it("disables transfer with an explanation while the asset is on loan", async () => {
    useAuthenticatedApi("operator");
    const loaned = onLoanItem();
    serve(loaned);
    renderApp(`/materiales/${loaned.id}`);
    const button = await screen.findByRole("button", { name: "Trasladar" });
    expect(button).toBeDisabled();
    expect(button).toHaveAccessibleDescription(/devuélvelo antes de trasladarlo/);
  });
});
