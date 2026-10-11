import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import type { Item } from "../lib/api/schemas";
import { assignment, consumable, item, maintenanceRecord, movement, page, stock } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const serve = (value: Item) => server.use(http.get(api(`/items/${value.id}`), () => HttpResponse.json(value)));

async function openTab(name: string, route: string) {
  const user = userEvent.setup();
  renderApp(route);
  await user.click(await screen.findByRole("tab", { name }));
  return user;
}

describe("history tab", () => {
  it("lists consumable movements newest first and never asks for custody", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable();
    serve(supply);
    server.use(
      http.get(api(`/items/${supply.id}/movements`), () =>
        HttpResponse.json(
          page([
            movement({ id: 3, type: "out", qty: -2, stock_before: 7, stock_after: 5, note: "Obra norte", created_at: "2026-10-05T10:00:00Z" }),
            movement({ id: 2, type: "in", qty: 5, stock_before: 2, stock_after: 7, created_at: "2026-10-01T10:00:00Z" }),
            movement({ id: 1, type: "opening", qty: 2, stock_before: 0, stock_after: 2, user_name: null, created_at: "2026-09-01T10:00:00Z" }),
          ]),
        ),
      ),
    );
    await openTab("Historial", `/materiales/${supply.id}`);

    const list = await screen.findByRole("list", { name: "Historial del material" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("Salida");
    expect(rows[0]).toHaveTextContent("-2 caja");
    expect(rows[0]).toHaveTextContent("Existencias totales: 7 → 5 caja");
    expect(rows[0]).toHaveTextContent("Obra norte");
    expect(rows[0]).toHaveTextContent("Ana Pérez");
    expect(rows[1]).toHaveTextContent("Entrada");
    expect(rows[2]).toHaveTextContent("Existencia inicial");
    expect(rows[2]).toHaveTextContent("Sistema");
  });

  it("shows where each consumable movement happened and renders quantity transfers", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable({ stocks: [stock(10, 3), stock(11, 4)] });
    serve(supply);
    server.use(
      http.get(api(`/items/${supply.id}/movements`), () =>
        HttpResponse.json(
          page([
            movement({ id: 4, type: "out", qty: -3, location_id: 11, stock_before: 10, stock_after: 7, created_at: "2026-10-08T10:00:00Z" }),
            movement({
              id: 3,
              type: "transfer",
              qty: 0,
              moved_qty: 2,
              location_id: null,
              from_location_id: 10,
              to_location_id: 11,
              stock_before: 10,
              stock_after: 10,
              created_at: "2026-10-07T10:00:00Z",
            }),
            movement({ id: 2, type: "in", qty: 5, location_id: 10, created_at: "2026-10-02T10:00:00Z" }),
            movement({ id: 1, type: "opening", qty: 5, location_id: 11, stock_before: 0, stock_after: 5, user_name: null, created_at: "2026-09-01T10:00:00Z" }),
          ]),
        ),
      ),
    );
    await openTab("Historial", `/materiales/${supply.id}`);

    const rows = within(await screen.findByRole("list", { name: "Historial del material" })).getAllByRole("listitem");
    expect(rows).toHaveLength(4);
    expect(rows[0]).toHaveTextContent("Salida");
    expect(rows[0]).toHaveTextContent("-3 caja en Bodega › Estante A");
    expect(rows[1]).toHaveTextContent("Traslado");
    expect(rows[1]).toHaveTextContent("2 caja");
    expect(rows[1]).toHaveTextContent("Bodega → Bodega › Estante A");
    expect(rows[1]).not.toHaveTextContent("Existencias totales");
    expect(rows[2]).toHaveTextContent("+5 caja en Bodega");
    expect(rows[3]).toHaveTextContent("Existencia inicial");
    expect(rows[3]).toHaveTextContent("+5 caja en Bodega › Estante A");
  });

  it("degrades gracefully for legacy movements without a location", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable();
    serve(supply);
    server.use(
      http.get(api(`/items/${supply.id}/movements`), () =>
        HttpResponse.json(
          page([
            movement({ id: 2, type: "transfer", qty: 0, moved_qty: 4, location_id: null, from_location_id: null, to_location_id: 10, created_at: "2026-10-03T10:00:00Z" }),
            movement({ id: 1, type: "in", qty: 4, location_id: null, created_at: "2026-10-01T10:00:00Z" }),
          ]),
        ),
      ),
    );
    await openTab("Historial", `/materiales/${supply.id}`);
    const rows = within(await screen.findByRole("list", { name: "Historial del material" })).getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("4 caja");
    expect(rows[0]).toHaveTextContent("Sin ubicación → Bodega");
    expect(rows[1]).toHaveTextContent("+4 caja");
    expect(rows[1]).not.toHaveTextContent(" en ");
  });

  it("merges custody events with transfer movements for assets", async () => {
    useAuthenticatedApi("viewer");
    const asset = item();
    serve(asset);
    server.use(
      http.get(api(`/items/${asset.id}/movements`), () =>
        HttpResponse.json(
          page([movement({ id: 5, item_id: asset.id, type: "transfer", qty: 0, from_location_id: 10, to_location_id: 11, created_at: "2026-10-06T10:00:00Z" })]),
        ),
      ),
      http.get(api(`/items/${asset.id}/custody`), () =>
        HttpResponse.json(
          page([
            assignment({
              id: 2,
              item_id: asset.id,
              assigned_at: "2026-10-01T10:00:00Z",
              returned_at: "2026-10-03T10:00:00Z",
              returned_to_name: "Estante A",
              returned_by_name: "Luis Soto",
              overdue: false,
              assign_note: "Para la obra",
            }),
          ]),
        ),
      ),
    );
    await openTab("Historial", `/materiales/${asset.id}`);

    const rows = within(await screen.findByRole("list", { name: "Historial del material" })).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("Traslado");
    expect(rows[0]).toHaveTextContent("Bodega → Bodega › Estante A");
    expect(rows[1]).toHaveTextContent("Devuelto por Juan Gómez");
    expect(rows[1]).toHaveTextContent("Luis Soto");
    expect(rows[2]).toHaveTextContent("Prestado a Juan Gómez");
    expect(rows[2]).toHaveTextContent("Para la obra");
  });

  it("loads older movements on demand", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable();
    serve(supply);
    const offsets: string[] = [];
    server.use(
      http.get(api(`/items/${supply.id}/movements`), ({ request }) => {
        const offset = new URL(request.url).searchParams.get("offset") ?? "0";
        offsets.push(offset);
        return HttpResponse.json(
          offset === "0"
            ? page([movement({ id: 2, note: "reciente", created_at: "2026-10-05T10:00:00Z" })], { total: 2, limit: 1 })
            : page([movement({ id: 1, note: "antiguo", created_at: "2026-09-01T10:00:00Z" })], { total: 2, limit: 1, offset: 1 }),
        );
      }),
    );
    const user = await openTab("Historial", `/materiales/${supply.id}`);
    await screen.findByText("“reciente”");
    expect(screen.queryByText("“antiguo”")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cargar más" }));
    expect(await screen.findByText("“antiguo”")).toBeInTheDocument();
    expect(offsets).toEqual(["0", "1"]);
    expect(screen.queryByRole("button", { name: "Cargar más" })).not.toBeInTheDocument();
  });

  it("shows an empty state and an error state with retry", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable();
    serve(supply);
    let failing = true;
    server.use(
      http.get(api(`/items/${supply.id}/movements`), () =>
        failing ? HttpResponse.json(errorBody("internal_error", "x"), { status: 500 }) : HttpResponse.json(page([])),
      ),
    );
    const user = await openTab("Historial", `/materiales/${supply.id}`);
    expect(await screen.findByRole("alert")).toHaveTextContent(/error en el servidor/i);
    failing = false;
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText("Sin movimientos")).toBeInTheDocument();
  });
});

describe("maintenance tab", () => {
  it("highlights the open failure and shows the overdue banner", async () => {
    useAuthenticatedApi("viewer");
    const asset = item({ status: "inoperative", maintenance_overdue: true, maintenance_due_at: "2026-09-30T00:00:00Z" });
    serve(asset);
    server.use(
      http.get(api(`/items/${asset.id}/maintenance`), () =>
        HttpResponse.json(
          page([
            maintenanceRecord({ id: 2, destination: "Taller interno" }),
            maintenanceRecord({
              id: 1,
              kind: "preventive",
              status: "closed",
              description: "Cambio de escobillas",
              cost: 120.5,
              next_due_at: "2026-12-01T00:00:00Z",
              closed_at: "2026-06-01T00:00:00Z",
              reported_at: "2026-06-01T00:00:00Z",
            }),
          ]),
        ),
      ),
    );
    await openTab("Mantenimiento", `/materiales/${asset.id}`);

    expect(await screen.findByText(/Mantenimiento vencido desde/)).toBeInTheDocument();
    const open = await screen.findByRole("listitem", { name: "Falla abierta" });
    expect(open).toHaveTextContent("No enciende");
    expect(open).toHaveTextContent("Taller interno");
    const closed = screen.getByText("Cambio de escobillas").closest("li")!;
    expect(closed).toHaveTextContent("Preventivo");
    expect(closed).toHaveTextContent("$120,50");
    expect(closed).not.toHaveAccessibleName("Falla abierta");
  });

  it("is not offered for consumables", async () => {
    useAuthenticatedApi("viewer");
    const supply = consumable();
    serve(supply);
    server.use(http.get(api(`/items/${supply.id}/movements`), () => HttpResponse.json(page([]))));
    renderApp(`/materiales/${supply.id}`);
    await screen.findByRole("heading", { name: "ISK-CON-001" });
    expect(screen.queryByRole("tab", { name: "Mantenimiento" })).not.toBeInTheDocument();
  });
});
