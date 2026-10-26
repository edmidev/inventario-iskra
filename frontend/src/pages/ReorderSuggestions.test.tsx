import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { page, supplier } from "../test/fixtures";
import { api, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const suggestions = {
  groups: [
    {
      supplier_id: 1,
      supplier_name: "Ferretería Sur",
      lines: [
        {
          item_id: "22222222-2222-2222-2222-222222222222",
          code: "ISK-CON-001",
          description: "Cinta aislante",
          unit: "rollo",
          stock_current: 1,
          stock_min: 5,
          pending_ordered: 2,
          suggested_qty: 7,
          last_unit_cost_minor: 1250,
          last_currency: "USD",
        },
      ],
    },
    {
      supplier_id: null,
      supplier_name: null,
      lines: [
        {
          item_id: "33333333-3333-3333-3333-333333333333",
          code: "ISK-CON-002",
          description: "Pegamento",
          unit: "tubo",
          stock_current: 0,
          stock_min: 3,
          pending_ordered: 0,
          suggested_qty: 6,
          last_unit_cost_minor: null,
          last_currency: null,
        },
      ],
    },
  ],
};

function serve(body: { groups: unknown[] } = suggestions) {
  server.use(
    http.get(api("/dashboard/reorder-suggestions"), () => HttpResponse.json(body)),
    http.get(api("/suppliers"), () => HttpResponse.json(page([supplier()]))),
  );
}

describe("reorder suggestions page", () => {
  it("lists groups with suggested quantity, pending and last cost", async () => {
    useAuthenticatedApi("operator");
    serve();
    renderApp("/ordenes-compra/sugerencias");
    const sur = await screen.findByRole("region", { name: "Ferretería Sur" });
    expect(within(sur).getByText("Pedir 7 rollo")).toBeInTheDocument();
    expect(within(sur).getByText(/2 en camino/)).toBeInTheDocument();
    expect(within(sur).getByText(/Último: US\$ 12\.50/)).toBeInTheDocument();
    const none = screen.getByRole("region", { name: "Sin proveedor habitual" });
    expect(within(none).getByText("Pedir 6 tubo")).toBeInTheDocument();
  });

  it("opens a prefilled draft for a group", async () => {
    useAuthenticatedApi("operator");
    serve();
    const user = userEvent.setup();
    renderApp("/ordenes-compra/sugerencias");
    await user.click(await screen.findByRole("button", { name: "Crear orden para Ferretería Sur" }));
    const dialog = await screen.findByRole("dialog", { name: "Nueva orden de compra" });
    expect(within(dialog).getByDisplayValue("7")).toBeInTheDocument();
    expect(within(dialog).getByDisplayValue("12.50")).toBeInTheDocument();
  });

  it("hides the create buttons from a viewer", async () => {
    useAuthenticatedApi("viewer");
    serve();
    renderApp("/ordenes-compra/sugerencias");
    await screen.findByRole("region", { name: "Ferretería Sur" });
    expect(screen.queryByRole("button", { name: /Crear orden/ })).not.toBeInTheDocument();
  });

  it("shows an empty state when nothing needs ordering", async () => {
    useAuthenticatedApi("operator");
    serve({ groups: [] });
    renderApp("/ordenes-compra/sugerencias");
    expect(await screen.findByText("Nada que pedir")).toBeInTheDocument();
  });
});
