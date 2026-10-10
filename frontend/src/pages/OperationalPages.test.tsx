import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { assignment, dueMaintenance, maintenanceRecord, page, stockAlert, summary } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

describe("stock alerts page", () => {
  const alerts = [
    stockAlert({ item_id: "a1", code: "ISK-CON-001", stock_current: 0, shortage: -5 }),
    stockAlert({ item_id: "a2", code: "ISK-CON-002", description: "Cinta", stock_current: 3, shortage: -2 }),
    stockAlert({ item_id: "a3", code: "ISK-CON-003", description: "Tornillos", stock_current: 5, shortage: 0 }),
  ];

  it("lists alerts with severity and restocks with one sheet", async () => {
    useAuthenticatedApi("operator");
    let body: unknown;
    server.use(
      http.get(api("/stock/alerts"), () => HttpResponse.json(page(alerts))),
      http.post(api("/items/a2/movements"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(
          { movement: { id: 1, item_id: "a2", type: "in", qty: 10, stock_before: 3, stock_after: 13, from_location_id: null, to_location_id: null, user_id: null, user_name: null, note: "", created_at: "2026-10-10T00:00:00Z" }, stock_current: 13, location_id: 10 },
          { status: 201 },
        );
      }),
    );
    const user = userEvent.setup();
    renderApp("/stock-bajo");
    const list = await screen.findByRole("list", { name: "Consumibles con stock bajo" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("Agotado");
    expect(rows[1]).toHaveTextContent("Bajo el mínimo");
    expect(rows[2]).toHaveTextContent("En el mínimo");
    expect(screen.getByText("3 consumibles necesitan reposición")).toBeInTheDocument();

    await user.click(within(rows[1]).getByRole("button", { name: "Entrada de ISK-CON-002" }));
    const dialog = await screen.findByRole("dialog", { name: "Entrada de existencias" });
    await user.click(within(dialog).getByRole("button", { name: "10" }));
    await user.click(within(dialog).getByRole("button", { name: "Registrar entrada" }));
    expect(await screen.findByText(/Entrada registrada. Existencias: 13 caja/)).toBeInTheDocument();
    expect(body).toEqual({ type: "in", qty: 10 });
  });

  it("is read-only for viewers and shows an empty state", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api("/stock/alerts"), () => HttpResponse.json(page(alerts))));
    renderApp("/stock-bajo");
    await screen.findByRole("list", { name: "Consumibles con stock bajo" });
    expect(screen.queryByRole("button", { name: /Entrada de/ })).not.toBeInTheDocument();
  });

  it("shows the empty state", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api("/stock/alerts"), () => HttpResponse.json(page([]))));
    renderApp("/stock-bajo");
    expect(await screen.findByText("Todo el stock está en orden")).toBeInTheDocument();
  });
});

describe("loans page", () => {
  const loans = [
    assignment({ id: 2, item_id: "i2", item_code: "ISK-HER-002", assignee_name: "Juan Gómez", overdue: true }),
    assignment({ id: 3, item_id: "i3", item_code: "ISK-HER-003", assignee_name: "Caja 4", overdue: false, due_at: null }),
  ];

  it("shows overdue loans first and returns one from the list", async () => {
    useAuthenticatedApi("operator");
    let returned = false;
    server.use(
      http.get(api("/custody/active"), () => HttpResponse.json(page(loans))),
      http.post(api("/items/i2/return"), () => {
        returned = true;
        return HttpResponse.json(assignment({ id: 2, returned_at: "2026-10-10T00:00:00Z" }));
      }),
    );
    const user = userEvent.setup();
    renderApp("/prestamos");
    const rows = within(await screen.findByRole("list", { name: "Préstamos activos" })).getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("ISK-HER-002");
    expect(rows[0]).toHaveTextContent("Vencido");
    expect(rows[1]).toHaveTextContent("Sin fecha de devolución");

    await user.click(within(rows[0]).getByRole("button", { name: "Devolver ISK-HER-002" }));
    const dialog = await screen.findByRole("dialog", { name: "Registrar devolución" });
    await user.click(within(dialog).getByRole("button", { name: "Registrar devolución" }));
    expect(await screen.findByText("ISK-HER-002 devuelto.")).toBeInTheDocument();
    expect(returned).toBe(true);
  });

  it("sends the overdue filter and the assignee filter to the API", async () => {
    useAuthenticatedApi("viewer");
    const seen: URLSearchParams[] = [];
    server.use(
      http.get(api("/custody/active"), ({ request }) => {
        seen.push(new URL(request.url).searchParams);
        return HttpResponse.json(page(loans));
      }),
    );
    const user = userEvent.setup();
    renderApp("/prestamos");
    await screen.findByRole("list", { name: "Préstamos activos" });
    expect(screen.queryByRole("button", { name: /Devolver/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Solo vencidos" }));
    await user.click(screen.getByRole("combobox", { name: "Filtrar por responsable" }));
    await user.click(await screen.findByRole("option", { name: "Juan Gómez" }));
    await screen.findByRole("list", { name: "Préstamos activos" });
    const last = seen.at(-1)!;
    expect(last.get("overdue")).toBe("true");
    expect(last.get("assignee_location_id")).toBe("12");
  });

  it("restores the overdue filter from the URL (dashboard link)", async () => {
    useAuthenticatedApi("viewer");
    const seen: string[] = [];
    server.use(
      http.get(api("/custody/active"), ({ request }) => {
        seen.push(new URL(request.url).search);
        return HttpResponse.json(page([]));
      }),
    );
    renderApp("/prestamos?overdue=true");
    expect(await screen.findByText("Sin resultados")).toBeInTheDocument();
    expect(seen[0]).toContain("overdue=true");
    expect(screen.getByRole("button", { name: "Solo vencidos" })).toHaveAttribute("aria-pressed", "true");
  });

  it("is linked from the dashboard cards", async () => {
    useAuthenticatedApi("viewer");
    server.use(
      http.get(api("/dashboard/summary"), () => HttpResponse.json(summary)),
      http.get(api("/custody/active"), () => HttpResponse.json(page(loans))),
    );
    const user = userEvent.setup();
    renderApp("/inicio");
    await user.click(await screen.findByText("Prestados"));
    expect(await screen.findByRole("heading", { name: "Préstamos" })).toBeInTheDocument();
  });

  it("reports a load error with retry", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api("/custody/active"), () => HttpResponse.json(errorBody("internal_error", "x"), { status: 500 })));
    renderApp("/prestamos");
    expect(await screen.findByRole("alert")).toHaveTextContent(/error en el servidor/i);
  });
});

describe("maintenance page", () => {
  it("lists open failures and closes one", async () => {
    useAuthenticatedApi("operator");
    let closed = false;
    server.use(
      http.get(api("/maintenance"), () => HttpResponse.json(page([maintenanceRecord({ id: 9, destination: "Taller interno" })]))),
      http.post(api("/maintenance/9/close"), () => {
        closed = true;
        return HttpResponse.json(maintenanceRecord({ id: 9, status: "closed" }));
      }),
    );
    const user = userEvent.setup();
    renderApp("/mantenimiento");
    const row = (await screen.findAllByRole("listitem"))[0];
    expect(row).toHaveTextContent("No enciende");
    expect(row).toHaveTextContent("Taller interno");
    await user.click(within(row).getByRole("button", { name: "Cerrar falla de ISK-HER-001" }));
    const dialog = await screen.findByRole("dialog", { name: "Cerrar falla" });
    await user.click(within(dialog).getByRole("button", { name: "Cerrar falla" }));
    expect(await screen.findByText("Falla de ISK-HER-001 cerrada.")).toBeInTheDocument();
    expect(closed).toBe(true);
  });

  it("switches to the due tab and changes the window", async () => {
    useAuthenticatedApi("viewer");
    const windows: string[] = [];
    server.use(
      http.get(api("/maintenance"), () => HttpResponse.json(page([]))),
      http.get(api("/maintenance/due"), ({ request }) => {
        windows.push(new URL(request.url).searchParams.get("within_days") ?? "");
        return HttpResponse.json(page([dueMaintenance()]));
      }),
    );
    const user = userEvent.setup();
    renderApp("/mantenimiento");
    expect(await screen.findByText("No hay fallas abiertas")).toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Por vencer" }));
    const list = await screen.findByRole("list", { name: "Mantenimientos por vencer" });
    expect(list).toHaveTextContent("ISK-HER-001");
    expect(list).toHaveTextContent("Vencido");
    await user.click(screen.getByRole("combobox", { name: "Plazo" }));
    await user.click(await screen.findByRole("option", { name: "Vencidos y próximos 30 días" }));
    await screen.findByRole("list", { name: "Mantenimientos por vencer" });
    expect(windows).toContain("0");
    expect(windows.at(-1)).toBe("30");
  });

  it("opens on the due tab from the dashboard link and hides actions for viewers", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api("/maintenance/due"), () => HttpResponse.json(page([dueMaintenance()]))));
    renderApp("/mantenimiento?tab=due");
    expect(await screen.findByRole("list", { name: "Mantenimientos por vencer" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Cerrar falla/ })).not.toBeInTheDocument();
  });
});

describe("more page", () => {
  it("links to the operational pages", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    const nav = await screen.findByRole("navigation", { name: "Operaciones" });
    expect(within(nav).getByRole("link", { name: /Stock bajo/ })).toHaveAttribute("href", "/stock-bajo");
    expect(within(nav).getByRole("link", { name: /Préstamos/ })).toHaveAttribute("href", "/prestamos");
    expect(within(nav).getByRole("link", { name: /Mantenimiento/ })).toHaveAttribute("href", "/mantenimiento");
  });
});
