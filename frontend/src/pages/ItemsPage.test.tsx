import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import type { Item } from "../lib/api/schemas";
import { consumable, item, onLoanItem, stock } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

function serveItems(handler: (params: URLSearchParams) => { data: Item[]; total?: number; offset?: number }) {
  const requests: URLSearchParams[] = [];
  server.use(
    http.get(api("/items"), ({ request }) => {
      const params = new URL(request.url).searchParams;
      requests.push(params);
      const result = handler(params);
      return HttpResponse.json({ data: result.data, total: result.total ?? result.data.length, limit: 25, offset: result.offset ?? 0 });
    }),
  );
  return requests;
}

describe("items list", () => {
  it("tells where a consumable is: one place by name, several by count, none as no stock", async () => {
    useAuthenticatedApi("viewer");
    serveItems(() => ({
      data: [
        consumable({ id: "c-1", code: "ISK-CON-001", stocks: [stock(10, 3)] }),
        consumable({ id: "c-2", code: "ISK-CON-002", stocks: [stock(10, 3), stock(11, 4)] }),
        consumable({ id: "c-3", code: "ISK-CON-003", stock_current: 0 }),
      ],
    }));
    renderApp("/materiales");
    const one = await screen.findByRole("link", { name: /ISK-CON-001/ });
    expect(within(one).getByText("Bodega")).toBeInTheDocument();
    const many = screen.getByRole("link", { name: /ISK-CON-002/ });
    expect(within(many).getByText(/7 caja/)).toBeInTheDocument();
    expect(within(many).getByText("en 2 ubicaciones")).toBeInTheDocument();
    expect(within(screen.getByRole("link", { name: /ISK-CON-003/ })).getByText("Sin existencias")).toBeInTheDocument();
  });

  it("renders item cards with status, availability and stock information", async () => {
    useAuthenticatedApi("viewer");
    serveItems(() => ({ data: [item(), onLoanItem(), consumable()] }));
    renderApp("/materiales");

    expect(await screen.findByText("Taladro inalámbrico")).toBeInTheDocument();
    const loan = screen.getByRole("link", { name: /ISK-HER-002/ });
    expect(within(loan).getByText("Prestado a Juan Gómez")).toBeInTheDocument();
    expect(within(loan).getByText("Vencido")).toBeInTheDocument();

    const stockCard = screen.getByRole("link", { name: /ISK-CON-001/ });
    expect(within(stockCard).getByText(/2 caja/)).toBeInTheDocument();
    expect(within(stockCard).getByText("Bodega")).toBeInTheDocument();
    expect(within(stockCard).getByText("Stock bajo")).toBeInTheDocument();

    const asset = screen.getByRole("link", { name: /ISK-HER-001/ });
    expect(within(asset).getByText("Disponible")).toBeInTheDocument();
    expect(within(asset).getByText("Herramientas")).toBeInTheDocument();
    expect(screen.getByText("3 de 3 materiales")).toBeInTheDocument();
  });

  it("reads filters from the URL and sends them to the API", async () => {
    useAuthenticatedApi("viewer");
    const requests = serveItems(() => ({ data: [item({ status: "in_repair", availability: "unavailable" })] }));
    renderApp("/materiales?status=in_repair&category_id=1&q=taladro&low_stock=true&sort=updated_at&order=desc");

    expect(await screen.findByText("Taladro inalámbrico")).toBeInTheDocument();
    const params = requests[0];
    expect(params.get("status")).toBe("in_repair");
    expect(params.get("category_id")).toBe("1");
    expect(params.get("q")).toBe("taladro");
    expect(params.get("low_stock")).toBe("true");
    expect(params.get("sort")).toBe("updated_at");
    expect(params.get("order")).toBe("desc");
    expect(params.get("limit")).toBe("25");
    expect(params.get("offset")).toBe("0");
    expect(screen.getByRole("searchbox", { name: "Buscar materiales" })).toHaveValue("taladro");
    expect(screen.getByRole("button", { name: "Quitar filtro: En reparación" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Quitar filtro: Herramientas" })).toBeInTheDocument();
  });

  it("searches after a pause in typing", async () => {
    useAuthenticatedApi("viewer");
    const requests = serveItems((params) => ({ data: params.get("q") === "sierra" ? [onLoanItem()] : [item(), onLoanItem()] }));
    const user = userEvent.setup();
    renderApp("/materiales");
    await screen.findByText("Taladro inalámbrico");

    await user.type(screen.getByRole("searchbox", { name: "Buscar materiales" }), "sierra");
    expect(await screen.findByText("1 de 1 materiales")).toBeInTheDocument();
    expect(screen.queryByText("Taladro inalámbrico")).not.toBeInTheDocument();
    // Typing six characters must not fire six requests.
    expect(requests.filter((p) => p.get("q")).length).toBe(1);
  });

  it("removes a filter chip", async () => {
    useAuthenticatedApi("viewer");
    const requests = serveItems(() => ({ data: [item()] }));
    const user = userEvent.setup();
    renderApp("/materiales?status=functional");
    await screen.findByText("Taladro inalámbrico");
    await user.click(screen.getByRole("button", { name: "Quitar filtro: Funcional" }));
    await screen.findByText("Taladro inalámbrico");
    expect(requests.at(-1)!.has("status")).toBe(false);
    expect(screen.queryByRole("list", { name: "Filtros activos" })).not.toBeInTheDocument();
  });

  it("applies filters chosen in the filters sheet", async () => {
    useAuthenticatedApi("viewer");
    const requests = serveItems(() => ({ data: [item()] }));
    const user = userEvent.setup();
    renderApp("/materiales");
    await screen.findByText("Taladro inalámbrico");

    await user.click(screen.getByRole("button", { name: "Filtros" }));
    const sheet = await screen.findByRole("dialog", { name: "Filtros" });
    await user.click(within(sheet).getByRole("combobox", { name: "Categoría" }));
    await user.click(await screen.findByRole("option", { name: "Consumibles" }));
    await user.click(within(sheet).getByRole("checkbox", { name: "Solo stock bajo" }));
    await user.click(within(sheet).getByRole("button", { name: "Aplicar" }));

    expect(await screen.findByRole("button", { name: "Quitar filtro: Consumibles" })).toBeInTheDocument();
    const last = requests.at(-1)!;
    expect(last.get("category_id")).toBe("2");
    expect(last.get("low_stock")).toBe("true");
  });

  it("loads more pages with limit/offset", async () => {
    useAuthenticatedApi("viewer");
    const many = (from: number, n: number) =>
      Array.from({ length: n }, (_, i) => item({ id: `id-${from + i}`, code: `ISK-HER-${String(from + i).padStart(3, "0")}`, description: `Item ${from + i}` }));
    const requests = serveItems((params) => {
      const offset = Number(params.get("offset"));
      return { data: offset === 0 ? many(1, 25) : many(26, 5), total: 30, offset };
    });
    const user = userEvent.setup();
    renderApp("/materiales");
    expect(await screen.findByText("25 de 30 materiales")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cargar más" }));
    expect(await screen.findByText("30 de 30 materiales")).toBeInTheDocument();
    expect(requests.at(-1)!.get("offset")).toBe("25");
    expect(screen.queryByRole("button", { name: "Cargar más" })).not.toBeInTheDocument();
  });

  it("shows an empty state that can clear the filters", async () => {
    useAuthenticatedApi("viewer");
    serveItems((params) => ({ data: params.get("q") ? [] : [item()] }));
    const user = userEvent.setup();
    renderApp("/materiales?q=zzz");
    expect(await screen.findByText("Sin resultados")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Quitar filtros" }));
    expect(await screen.findByText("Taladro inalámbrico")).toBeInTheDocument();
  });

  it("shows an error state with retry", async () => {
    useAuthenticatedApi("viewer");
    let failing = true;
    server.use(
      http.get(api("/items"), () =>
        failing
          ? HttpResponse.json(errorBody("validation_failed", "bad", { status: "invalid" }), { status: 422 })
          : HttpResponse.json({ data: [item()], total: 1, limit: 25, offset: 0 }),
      ),
    );
    const user = userEvent.setup();
    renderApp("/materiales");
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText("Taladro inalámbrico")).toBeInTheDocument();
  });
});
