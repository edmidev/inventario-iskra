import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppProviders, createQueryClient } from "../providers";
import { api } from "../test/api";
import { server } from "../test/server";
import { FinishedStockPage } from "./FinishedStockPage";

describe("finished product stock", () => {
  it("shows available serialized units and filters delivered history", async () => {
    const user = userEvent.setup();
    const statuses: string[] = [];
    server.use(http.get(api("/finished-products"), ({ request }) => {
      const status = new URL(request.url).searchParams.get("status") ?? "";
      statuses.push(status);
      const row = {
        id: 1, code: "PRD-000001", execution_id: 27, project_name: "Marmita", produced_at: "2026-01-01T00:00:00Z",
        status: status === "delivered" ? "delivered" : "available",
        ...(status === "delivered" ? { receipt: "DEL-000001", customer_name: "Cliente Sur", delivered_at: "2026-01-02T12:00:00Z" } : {}),
      };
      return HttpResponse.json({ data: [row], total: 1, limit: 20, offset: 0 });
    }));
    render(<AppProviders client={createQueryClient()}><MemoryRouter><FinishedStockPage /></MemoryRouter></AppProviders>);
    expect(await screen.findByText("PRD-000001")).toBeInTheDocument();
    expect(screen.getByText("Disponible")).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Estado del producto"), "delivered");
    expect(await screen.findByText("Cliente Sur")).toBeInTheDocument();
    expect(screen.getByText("DEL-000001")).toBeInTheDocument();
    expect(statuses).toContain("delivered");
    await user.selectOptions(screen.getByLabelText("Estado del producto"), "all");
    expect(await screen.findByText("Disponible")).toBeInTheDocument();
    expect(statuses).toContain("");
  });
});
