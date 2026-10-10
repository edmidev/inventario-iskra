import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { item, summary } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

describe("dashboard", () => {
  it("renders stat cards and alert lists from the summary", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api("/dashboard/summary"), () => HttpResponse.json(summary)));
    renderApp("/inicio");

    expect(await screen.findByText("Total de materiales")).toBeInTheDocument();
    const cards = screen.getByText("Total de materiales").closest("a")!;
    expect(cards).toHaveTextContent("48");
    expect(screen.getByText("Disponibles").closest("a")).toHaveTextContent("30");
    expect(screen.getByText("Prestados").parentElement).toHaveTextContent("4");
    expect(screen.getByText("En reparación o inoperativos").parentElement).toHaveTextContent("5");

    const lowStock = screen.getByRole("region", { name: "Stock bajo" });
    expect(within(lowStock).getByText("ISK-CON-001")).toBeInTheDocument();
    expect(within(lowStock).getByText("2 / 5 caja")).toBeInTheDocument();

    const loans = screen.getByRole("region", { name: "Préstamos vencidos" });
    expect(within(loans).getByText("Juan Gómez")).toBeInTheDocument();
    expect(within(loans).getByText(/Venció/)).toBeInTheDocument();

    const maintenance = screen.getByRole("region", { name: "Mantenimiento vencido" });
    expect(within(maintenance).getByText("ISK-HER-001")).toBeInTheDocument();
  });

  it("opens the item detail when a row is tapped", async () => {
    useAuthenticatedApi("viewer");
    server.use(
      http.get(api("/dashboard/summary"), () => HttpResponse.json(summary)),
      http.get(api("/items/33333333-3333-3333-3333-333333333333"), () =>
        HttpResponse.json(item({ id: "33333333-3333-3333-3333-333333333333", code: "ISK-HER-002", description: "Sierra circular" })),
      ),
    );
    const user = userEvent.setup();
    renderApp("/inicio");
    const loans = await screen.findByRole("region", { name: "Préstamos vencidos" });
    await user.click(within(loans).getByRole("link", { name: /ISK-HER-002/ }));
    expect(await screen.findByRole("heading", { name: "ISK-HER-002" })).toBeInTheDocument();
  });

  it("shows friendly empty messages when nothing needs attention", async () => {
    useAuthenticatedApi("viewer");
    server.use(
      http.get(api("/dashboard/summary"), () =>
        HttpResponse.json({
          ...summary,
          low_stock: { count: 0, top: [] },
          loans: { active: 0, overdue: 0, overdue_top: [] },
          maintenance: { open_failures: 0, overdue: 0, overdue_top: [] },
        }),
      ),
    );
    renderApp("/inicio");
    expect(await screen.findByText("Todo el stock está en orden.")).toBeInTheDocument();
    expect(screen.getByText("No hay préstamos vencidos.")).toBeInTheDocument();
    expect(screen.getByText("No hay mantenimientos vencidos.")).toBeInTheDocument();
  });

  it("shows an error with retry and recovers", async () => {
    useAuthenticatedApi("viewer");
    let failing = true;
    server.use(
      http.get(api("/dashboard/summary"), () =>
        failing ? HttpResponse.json(errorBody("internal_error", "boom"), { status: 500 }) : HttpResponse.json(summary),
      ),
    );
    const user = userEvent.setup();
    renderApp("/inicio");
    expect(await screen.findByRole("alert")).toHaveTextContent(/error en el servidor/i);
    failing = false;
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText("Total de materiales")).toBeInTheDocument();
  });
});
