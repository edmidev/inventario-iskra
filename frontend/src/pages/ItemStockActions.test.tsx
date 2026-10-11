import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { dashboardKey } from "../features/dashboard/api/useDashboardSummary";
import type { Item, ItemStock } from "../lib/api/schemas";
import { consumable, item, movement, onLoanItem, stock, summary } from "../test/fixtures";
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

const result = (stocks: ItemStock[], qty: number) => ({
  movement: movement({ qty, stock_after: stocks.reduce((sum, row) => sum + row.quantity, 0) }),
  stock_current: stocks.reduce((sum, row) => sum + row.quantity, 0),
  location_id: null,
  stocks,
});

/** Records the last movement body and answers 201 with the given stock breakdown. */
function recordMovements(value: Item, after: ItemStock[], qty = 0) {
  const seen: { body?: unknown } = {};
  server.use(
    http.post(api(`/items/${value.id}/movements`), async ({ request }) => {
      seen.body = await request.json();
      return HttpResponse.json(result(after, qty), { status: 201 });
    }),
  );
  return seen;
}

/** Consumable with 5 at Bodega (10) and 3 at Estante A (11). */
const twoPlaces = () => consumable({ stocks: [stock(10, 5), stock(11, 3)], stock_min: 4, low_stock: false });

describe("stock actions", () => {
  it("hides every action from viewers but still shows the stock by location", async () => {
    useAuthenticatedApi("viewer");
    const supply = twoPlaces();
    serve(supply);
    renderApp(`/materiales/${supply.id}`);
    await screen.findByRole("heading", { name: "ISK-CON-001" });
    expect(screen.getByRole("region", { name: "Stock por ubicación" })).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Acciones" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Entrada" })).not.toBeInTheDocument();
  });

  it("shows the actions to operators", async () => {
    useAuthenticatedApi("operator");
    const supply = twoPlaces();
    serve(supply);
    renderApp(`/materiales/${supply.id}`);
    const actions = await screen.findByRole("region", { name: "Acciones" });
    for (const name of ["Entrada", "Salida", "Ajuste", "Trasladar"]) {
      expect(within(actions).getByRole("button", { name })).toBeInTheDocument();
    }
  });

  describe("in a single location", () => {
    it("preselects the only location, registers an exit with a quick quantity and refreshes item and dashboard", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 12, stock_min: 5, low_stock: false });
      const state = serve(supply);
      let body: unknown;
      server.use(
        http.post(api(`/items/${supply.id}/movements`), async ({ request }) => {
          body = await request.json();
          state.current = consumable({ stock_current: 7, stock_min: 5, low_stock: false });
          return HttpResponse.json(result([stock(10, 7)], -5), { status: 201 });
        }),
      );
      const user = userEvent.setup();
      const { client } = renderApp(`/materiales/${supply.id}`);
      await client.prefetchQuery({ queryKey: dashboardKey, queryFn: () => summary });

      await user.click(await screen.findByRole("button", { name: "Salida" }));
      const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
      expect(within(dialog).getByRole("combobox", { name: /Ubicación de la salida/ })).toHaveTextContent("Bodega · 12 caja");
      expect(within(dialog).getByText(/En Bodega:/)).toHaveTextContent("12 caja");
      await user.click(within(dialog).getByRole("button", { name: "5" }));
      expect(within(dialog).getByText(/Quedarán/)).toHaveTextContent("7 caja en Bodega · total 7 caja");
      await user.click(within(dialog).getByRole("button", { name: "Registrar salida" }));

      expect(await screen.findByText(/Salida registrada en Bodega. Existencias: 7 caja/)).toBeInTheDocument();
      expect(body).toEqual({ type: "out", location_id: 10, qty: 5 });
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
      await waitFor(() => expect(state.gets).toBeGreaterThan(1));
      expect(client.getQueryState(dashboardKey)?.isInvalidated).toBe(true);
      const rows = await screen.findByRole("list", { name: "Existencias por ubicación" });
      await waitFor(() => expect(within(rows).getByText("7 caja")).toBeInTheDocument());
    });

    it("steps the quantity with the large buttons and warns when the total ends up low", async () => {
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
      expect(within(dialog).getByText("El total quedará en o por debajo del stock mínimo.")).toBeInTheDocument();
    });

    it("requires a note for an adjustment and sends the absolute count at the location", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 4 });
      serve(supply);
      const seen = recordMovements(supply, [stock(10, 9)], 5);
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
      expect(seen.body).toBeUndefined();
      await user.type(within(dialog).getByLabelText(/Motivo del ajuste/), "Conteo físico");
      await user.click(within(dialog).getByRole("button", { name: "Guardar ajuste" }));
      expect(await screen.findByText(/Ajuste registrado en Bodega/)).toBeInTheDocument();
      expect(seen.body).toEqual({ type: "adjust", location_id: 10, qty: 9, note: "Conteo físico" });
    });
  });

  describe("in several locations", () => {
    it("asks for the location first and does not submit without one", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      const seen = recordMovements(supply, []);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Entrada" }));
      const dialog = await screen.findByRole("dialog", { name: "Entrada de existencias" });
      expect(within(dialog).getByRole("combobox", { name: /Ubicación/ })).toHaveTextContent("Elige una ubicación");
      await user.click(within(dialog).getByRole("button", { name: "Registrar entrada" }));
      expect(await within(dialog).findByText("Elige la ubicación.")).toBeInTheDocument();
      expect(seen.body).toBeUndefined();
    });

    it("shows what the chosen location holds and what will remain after a salida", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      const seen = recordMovements(supply, [stock(10, 5), stock(11, 1)], -2);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Salida" }));
      const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de la salida/ }));
      // Salida only offers places that hold stock, each with its quantity.
      expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual(["Bodega · 5 caja", "Bodega › Estante A · 3 caja"]);
      await user.click(screen.getByRole("option", { name: /Estante A/ }));
      expect(within(dialog).getByText(/En Bodega › Estante A:/)).toHaveTextContent("3 caja");
      await user.click(within(dialog).getByRole("button", { name: "Aumentar cantidad" }));
      expect(within(dialog).getByText(/Quedarán/)).toHaveTextContent("1 caja en Bodega › Estante A · total 6 caja");
      await user.click(within(dialog).getByRole("button", { name: "Registrar salida" }));
      expect(await screen.findByText(/Salida registrada en Bodega › Estante A. Existencias: 6 caja/)).toBeInTheDocument();
      expect(seen.body).toEqual({ type: "out", location_id: 11, qty: 2 });
    });

    it("warns and blocks a salida above what the location holds", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      const seen = recordMovements(supply, []);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Salida" }));
      const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de la salida/ }));
      await user.click(screen.getByRole("option", { name: /Estante A/ }));
      await user.clear(within(dialog).getByLabelText(/Cantidad que sale/));
      await user.type(within(dialog).getByLabelText(/Cantidad que sale/), "4");
      expect(within(dialog).getByText("La salida supera las existencias de Bodega › Estante A (3 caja).")).toBeInTheDocument();
      expect(within(dialog).getByRole("button", { name: "Registrar salida" })).toBeDisabled();
      expect(seen.body).toBeUndefined();
    });

    it("lets an entrada start stock in a location that has none yet and lists every location", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      const seen = recordMovements(supply, [stock(10, 5), stock(11, 3), stock(12, 4)], 4);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Entrada" }));
      const dialog = await screen.findByRole("dialog", { name: "Entrada de existencias" });
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación/ }));
      expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual([
        "Bodega · 5 caja",
        "Bodega › Estante A · 3 caja",
        "Juan Gómez",
      ]);
      await user.click(screen.getByRole("option", { name: "Juan Gómez" }));
      expect(within(dialog).getByText(/En Juan Gómez:/)).toHaveTextContent("0 caja");
      await user.clear(within(dialog).getByLabelText(/Cantidad que entra/));
      await user.type(within(dialog).getByLabelText(/Cantidad que entra/), "4");
      expect(within(dialog).getByText(/Quedarán/)).toHaveTextContent("4 caja en Juan Gómez · total 12 caja");
      await user.click(within(dialog).getByRole("button", { name: "Registrar entrada" }));
      expect(await screen.findByText(/Entrada registrada en Juan Gómez/)).toBeInTheDocument();
      expect(seen.body).toEqual({ type: "in", location_id: 12, qty: 4 });
    });

    it("preselects the location used last time", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      recordMovements(supply, [stock(10, 5), stock(11, 4)], 1);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Entrada" }));
      let dialog = await screen.findByRole("dialog", { name: "Entrada de existencias" });
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación/ }));
      await user.click(screen.getByRole("option", { name: /Estante A/ }));
      await user.click(within(dialog).getByRole("button", { name: "Registrar entrada" }));
      await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());

      await user.click(screen.getByRole("button", { name: "Salida" }));
      dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
      expect(within(dialog).getByRole("combobox", { name: /Ubicación de la salida/ })).toHaveTextContent("Bodega › Estante A");
    });

    it("resets the counted quantity to what the chosen location holds when adjusting", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Ajuste" }));
      const dialog = await screen.findByRole("dialog", { name: "Ajuste de inventario" });
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación/ }));
      await user.click(screen.getByRole("option", { name: /Estante A/ }));
      expect(within(dialog).getByLabelText(/Cantidad contada/)).toHaveValue(3);
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación/ }));
      await user.click(screen.getByRole("option", { name: /^Bodega · 5/ }));
      expect(within(dialog).getByLabelText(/Cantidad contada/)).toHaveValue(5);
    });
  });

  describe("errors", () => {
    it("names the location when the server says there is not enough stock", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 3 });
      serve(supply);
      server.use(
        http.post(api(`/items/${supply.id}/movements`), () =>
          HttpResponse.json(errorBody("insufficient_stock", 'only 3 available in "Bodega", 4 requested'), { status: 409 }),
        ),
      );
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Salida" }));
      const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
      await user.click(within(dialog).getByRole("button", { name: "Registrar salida" }));
      expect(await within(dialog).findByRole("alert")).toHaveTextContent("No hay existencias suficientes en Bodega: hay 3 caja.");
      expect(screen.getByRole("dialog")).toBeInTheDocument();
    });

    it("maps a location_required answer onto the location field", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 3 });
      serve(supply);
      server.use(
        http.post(api(`/items/${supply.id}/movements`), () =>
          HttpResponse.json(errorBody("location_required", "x", { location_id: "required: the item is stocked in several locations" }), { status: 422 }),
        ),
      );
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Entrada" }));
      const dialog = await screen.findByRole("dialog", { name: "Entrada de existencias" });
      await user.click(within(dialog).getByRole("button", { name: "Registrar entrada" }));
      expect(await within(dialog).findByText("Elige la ubicación.")).toBeInTheDocument();
      expect(within(dialog).queryByText(/stocked in several/)).not.toBeInTheDocument();
    });

    it("maps 422 field errors onto the note", async () => {
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

    it("explains there is nothing to take out when the item has no stock", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 0 });
      serve(supply);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Salida" }));
      const dialog = await screen.findByRole("dialog", { name: "Salida de existencias" });
      expect(within(dialog).getByRole("alert")).toHaveTextContent("no hay nada que sacar");
      expect(within(dialog).getByRole("button", { name: "Registrar salida" })).toBeDisabled();
    });
  });

  describe("consumable transfer", () => {
    it("moves a partial quantity between two locations and shows the summary", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      const seen = recordMovements(supply, [stock(10, 3), stock(11, 3), stock(12, 2)], 0);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Trasladar" }));
      const dialog = await screen.findByRole("dialog", { name: "Trasladar existencias" });

      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de origen/ }));
      expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual(["Bodega · 5 caja", "Bodega › Estante A · 3 caja"]);
      await user.click(screen.getByRole("option", { name: /^Bodega · 5/ }));
      expect(within(dialog).getByText("Disponible en Bodega: 5 caja.")).toBeInTheDocument();

      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de destino/ }));
      // The origin cannot be the destination.
      expect(screen.queryByRole("option", { name: /^Bodega · 5/ })).not.toBeInTheDocument();
      await user.click(screen.getByRole("option", { name: "Juan Gómez" }));

      await user.click(within(dialog).getByRole("button", { name: "Aumentar cantidad" }));
      expect(within(dialog).getByText("Mover 2 caja de Bodega a Juan Gómez")).toBeInTheDocument();
      expect(within(dialog).getByText("Quedarán 3 caja en Bodega y 2 caja en Juan Gómez.")).toBeInTheDocument();
      await user.click(within(dialog).getByRole("button", { name: "Trasladar" }));

      expect(await screen.findByText("Trasladado: 2 caja de Bodega a Juan Gómez.")).toBeInTheDocument();
      expect(seen.body).toEqual({ type: "transfer", from_location_id: 10, to_location_id: 12, qty: 2 });
    });

    it("limits the quantity to what the origin holds", async () => {
      useAuthenticatedApi("operator");
      const supply = twoPlaces();
      serve(supply);
      const seen = recordMovements(supply, []);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Trasladar" }));
      const dialog = await screen.findByRole("dialog", { name: "Trasladar existencias" });
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de origen/ }));
      await user.click(screen.getByRole("option", { name: /Estante A/ }));
      const qty = within(dialog).getByLabelText(/Cantidad a trasladar/);
      await user.clear(qty);
      await user.type(qty, "9");
      expect(qty).toHaveValue(3);
      expect(within(dialog).getByRole("button", { name: "Aumentar cantidad" })).toBeDisabled();
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de origen/ }));
      await user.click(screen.getByRole("option", { name: /^Bodega · 5/ }));
      // Changing the origin keeps the quantity within bounds and does not submit anything yet.
      expect(qty).toHaveValue(3);
      expect(seen.body).toBeUndefined();
    });

    it("preselects the origin when the item is in one place only and asks for a destination", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 6 });
      serve(supply);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Trasladar" }));
      const dialog = await screen.findByRole("dialog", { name: "Trasladar existencias" });
      expect(within(dialog).getByRole("combobox", { name: /Ubicación de origen/ })).toHaveTextContent("Bodega · 6 caja");
      await user.click(within(dialog).getByRole("button", { name: "Trasladar" }));
      expect(await within(dialog).findByText("Elige la ubicación de destino.")).toBeInTheDocument();
    });

    it("maps server errors: field messages and insufficient stock at the origin", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 6 });
      serve(supply);
      let calls = 0;
      server.use(
        http.post(api(`/items/${supply.id}/movements`), () => {
          calls += 1;
          return calls === 1
            ? HttpResponse.json(errorBody("validation_failed", "x", { to_location_id: "location does not exist" }), { status: 422 })
            : HttpResponse.json(errorBody("insufficient_stock", "x"), { status: 409 });
        }),
      );
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Trasladar" }));
      const dialog = await screen.findByRole("dialog", { name: "Trasladar existencias" });
      await user.click(within(dialog).getByRole("combobox", { name: /Ubicación de destino/ }));
      await user.click(screen.getByRole("option", { name: /Estante A/ }));
      await user.click(within(dialog).getByRole("button", { name: "Trasladar" }));
      expect(await within(dialog).findByText("location does not exist")).toBeInTheDocument();
      await user.click(within(dialog).getByRole("button", { name: "Trasladar" }));
      expect(await within(dialog).findByRole("alert")).toHaveTextContent("No hay existencias suficientes en Bodega: hay 6 caja.");
    });

    it("explains there is nothing to move when the item has no stock", async () => {
      useAuthenticatedApi("operator");
      const supply = consumable({ stock_current: 0 });
      serve(supply);
      const user = userEvent.setup();
      renderApp(`/materiales/${supply.id}`);
      await user.click(await screen.findByRole("button", { name: "Trasladar" }));
      const dialog = await screen.findByRole("dialog", { name: "Trasladar existencias" });
      expect(within(dialog).getByRole("alert")).toHaveTextContent("no hay nada que trasladar");
      expect(within(dialog).getByRole("button", { name: "Trasladar" })).toBeDisabled();
    });
  });

  describe("asset transfer", () => {
    it("transfers an asset to another location", async () => {
      useAuthenticatedApi("operator");
      const asset = item();
      serve(asset);
      let body: unknown;
      server.use(
        http.post(api(`/items/${asset.id}/movements`), async ({ request }) => {
          body = await request.json();
          return HttpResponse.json(
            { movement: movement({ type: "transfer", qty: 0, location_id: null }), stock_current: 1, location_id: 10, stocks: [] },
            { status: 201 },
          );
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
});
