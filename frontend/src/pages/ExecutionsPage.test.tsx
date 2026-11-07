import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { api, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const project = { id: 4, name: "Marmita", description: "Acero inoxidable", characteristics: ["Aislada"], consumables: [{ item_id: "item-1", quantity: 1.25 }], tools: ["tool-1"], estimated_days: 12, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" };
const customers = [
  { id: 8, name: "Cliente Norte", identification: "", contact: "", notes: "", active: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 9, name: "Cliente Sur", identification: "", contact: "", notes: "", active: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

describe("execution registration", () => {
  it("submits a template snapshot with multiple customers, workers, and an adjusted duration", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [], total: 0, limit: 25, offset: 0 })),
      http.get(api("/projects"), () => HttpResponse.json({ data: [project], total: 1, limit: 25, offset: 0 })),
      http.get(api("/customers"), () => HttpResponse.json({ data: customers, total: 2, limit: 100, offset: 0 })),
      http.get(api("/executions/assignable-users"), () => HttpResponse.json({ data: [{ id: "user-a", name: "Ana" }, { id: "user-b", name: "Luis" }] })),
    );
    let body: unknown;
    server.use(http.post(api("/executions"), async ({ request }) => {
      body = await request.json();
      return HttpResponse.json({
        id: 1, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: ["Aislada"],
        consumables: [{ item_id: "item-1", quantity_per_unit: 1.25 }], tools: ["tool-1"], template_estimated_days: 12,
        units: 3, estimated_days: 8, customer_ids: [8, 9], principal_user_id: "user-a", worker_ids: ["user-b"],
        status: "registered", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      }, { status: 201 });
    }));
    renderApp("/ejecuciones");
    await user.click(await screen.findByRole("button", { name: "Nueva ejecución" }));
    await user.selectOptions(screen.getByLabelText("Proyecto"), "4");
    await user.clear(screen.getByLabelText("Unidades"));
    await user.type(screen.getByLabelText("Unidades"), "3");
    await user.clear(screen.getByLabelText("Días estimados"));
    await user.type(screen.getByLabelText("Días estimados"), "8");
    await user.click(screen.getByLabelText("Cliente Norte"));
    await user.type(screen.getByLabelText("Buscar clientes"), "Sur");
    await user.click(screen.getByLabelText("Cliente Sur"));
    await user.selectOptions(screen.getByLabelText("Responsable principal"), "user-a");
    await user.click(screen.getByLabelText("Luis"));
    await user.click(screen.getByRole("button", { name: "Registrar ejecución" }));
    await screen.findByText("Ejecución registrada con un snapshot independiente del proyecto.");
    await waitFor(() => expect(body).toEqual({ project_id: 4, units: 3, estimated_days: 8, customer_ids: [8, 9], principal_user_id: "user-a", worker_ids: ["user-b"] }));
  });

  it("starts a registered job even when inventory is not yet complete", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    const execution = {
      id: 3, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, units: 1, estimated_days: 12, customer_ids: [8], principal_user_id: "user-a", worker_ids: [],
      status: "registered", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [execution], total: 1, limit: 25, offset: 0 })),
      http.post(api("/executions/3/start"), () => HttpResponse.json({ ...execution, status: "in_progress" })),
    );
    renderApp("/ejecuciones");
    await user.click(await screen.findByRole("button", { name: "Iniciar" }));
    expect(await screen.findByText("Ejecución iniciada; el inventario puede completarse durante el trabajo.")).toBeInTheDocument();
  });
});
