import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it, vi } from "vitest";
import { api, renderApp, useAuthenticatedApi } from "../test/render";
import { session } from "../test/fixtures";
import { server } from "../test/server";

const project = { id: 4, name: "Marmita", description: "Acero inoxidable", characteristics: ["Aislada"], consumables: [{ item_id: "item-1", quantity: 1.25 }], tools: ["tool-1"], estimated_days: 12, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" };
const customers = [
  { id: 8, name: "Cliente Norte", identification: "", contact: "", notes: "", active: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: 9, name: "Cliente Sur", identification: "", contact: "", notes: "", active: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];
const extraConsumable = {
  id: "00000000-0000-4000-8000-000000000099", code: "INS-1", description: "Tornillo inoxidable", category_id: 1,
  kind: "consumable", status: "functional", availability: "available", unit: "unidad", stock_current: 10, stock_min: 0,
  stocks: [], low_stock: false, on_loan: false, location_id: null, last_maintenance_at: null, maintenance_interval_days: null,
  maintenance_due_at: null, maintenance_overdue: false, attributes: {}, notes: "", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
};
const extraTool = {
  ...extraConsumable, id: "00000000-0000-4000-8000-000000000098", code: "HER-1", description: "Soldadora TIG", kind: "asset",
};
const itemList = () => {
  return { data: [extraConsumable, extraTool], total: 2, limit: 100, offset: 0 };
};

describe("execution registration", () => {
  it("submits a template snapshot with multiple customers, workers, and an adjusted duration", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [], total: 0, limit: 25, offset: 0 })),
      http.get(api("/projects"), () => HttpResponse.json({ data: [project], total: 1, limit: 25, offset: 0 })),
      http.get(api("/customers"), () => HttpResponse.json({ data: customers, total: 2, limit: 100, offset: 0 })),
      http.get(api("/executions/assignable-users"), () => HttpResponse.json({ data: [{ id: "user-a", name: "Ana" }, { id: "user-b", name: "Luis" }] })),
      http.get(api("/items"), () => HttpResponse.json(itemList())),
    );
    let body: unknown;
    server.use(http.post(api("/executions"), async ({ request }) => {
      body = await request.json();
      return HttpResponse.json({
        id: 1, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: ["Aislada"],
        consumables: [{ item_id: "item-1", quantity_per_unit: 1.25 }], tools: ["tool-1", extraTool.id], template_estimated_days: 12,
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
    await user.click(screen.getByRole("button", { name: "Agregar consumible" }));
    await user.selectOptions(screen.getByLabelText("Insumo adicional 1"), extraConsumable.id);
    await user.type(screen.getByLabelText("Cantidad total 1"), "2.5");
    await user.selectOptions(screen.getByLabelText("Herramienta adicional"), extraTool.id);
    await user.click(screen.getByRole("button", { name: "Agregar herramienta" }));
    await user.click(screen.getByLabelText("Cliente Norte"));
    await user.type(screen.getByLabelText("Buscar clientes"), "Sur");
    await user.click(screen.getByLabelText("Cliente Sur"));
    await user.selectOptions(screen.getByLabelText("Responsable principal"), "user-a");
    await user.click(screen.getByLabelText("Luis"));
    await user.click(screen.getByRole("button", { name: "Registrar ejecución" }));
    await screen.findByText("Ejecución registrada con un snapshot independiente del proyecto.");
    await waitFor(() => expect(body).toEqual({ project_id: 4, units: 3, estimated_days: 8, customer_ids: [8, 9], principal_user_id: "user-a", worker_ids: ["user-b"], additional_materials: [{ item_id: extraConsumable.id, quantity: 2.5 }], additional_tools: [extraTool.id] }));
  });

  it("retries an uncertain supplemental addition with the same idempotency key", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    const execution = {
      id: 3, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, units: 4, estimated_days: 12, materials: [], customer_ids: [8], principal_user_id: "user-a", worker_ids: [],
      status: "in_progress", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    const requestIDs: string[] = [];
    let attempts = 0;
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [execution], total: 1, limit: 25, offset: 0 })),
      http.get(api("/items"), () => HttpResponse.json({ data: [extraConsumable], total: 1, limit: 100, offset: 0 })),
      http.post(api("/executions/3/materials"), async ({ request }) => {
        const body = await request.json() as { request_id: string };
        requestIDs.push(body.request_id);
        attempts++;
        if (attempts === 1) return HttpResponse.error();
        return HttpResponse.json({ ...execution, materials: [{ item_id: extraConsumable.id, required_quantity: 2.5, reserved_quantity: 1, delivered_quantity: 0, returned_quantity: 0, deficit_quantity: 1.5 }] });
      }),
    );
    renderApp("/ejecuciones");
    await screen.findByRole("option", { name: /Tornillo inoxidable/ });
    await user.selectOptions(screen.getByLabelText("Consumible adicional"), extraConsumable.id);
    await user.type(screen.getByLabelText("Cantidad total adicional"), "2.5");
    await user.click(screen.getByRole("button", { name: "Agregar insumo" }));
    await waitFor(() => expect(attempts).toBe(1));
    await user.click(screen.getByRole("button", { name: "Agregar insumo" }));
    await screen.findByText("Material adicional agregado a la ejecución.");
    expect(requestIDs).toHaveLength(2);
    expect(requestIDs[0]).toMatch(/^[0-9a-f-]{36}$/);
    expect(requestIDs[1]).toBe(requestIDs[0]);
  });

  it("adds an informational tool during execution with a stable retry key", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    const execution = {
      id: 3, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, units: 2, estimated_days: 12, materials: [], customer_ids: [8], principal_user_id: "user-a", worker_ids: [],
      status: "in_progress", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    const requestIDs: string[] = [];
    let attempts = 0;
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [execution], total: 1, limit: 25, offset: 0 })),
      http.get(api("/items"), () => HttpResponse.json(itemList())),
      http.post(api("/executions/3/tools"), async ({ request }) => {
        const body = await request.json() as { request_id: string; item_id: string };
        requestIDs.push(body.request_id);
        attempts++;
        if (attempts === 1) return HttpResponse.error();
        return HttpResponse.json({ ...execution, tools: [extraTool.id] });
      }),
    );
    renderApp("/ejecuciones");
    await screen.findByRole("option", { name: /Soldadora TIG/ });
    await user.selectOptions(await screen.findByLabelText("Herramienta adicional (informativa)"), extraTool.id);
    await user.click(screen.getByRole("button", { name: "Agregar herramienta" }));
    await waitFor(() => expect(attempts).toBe(1));
    await user.click(screen.getByRole("button", { name: "Agregar herramienta" }));
    await screen.findByText("Herramienta agregada a la ejecución.");
    expect(requestIDs).toHaveLength(2);
    expect(requestIDs[0]).toMatch(/^[0-9a-f-]{36}$/);
    expect(requestIDs[1]).toBe(requestIDs[0]);
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

  it("delivers reserved material from a selected physical location", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    const execution = {
      id: 3, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: [],
      consumables: [{ item_id: "item-1", quantity_per_unit: 1.25 }], tools: [], template_estimated_days: 12,
      units: 2, estimated_days: 12, materials: [{ item_id: "item-1", required_quantity: 2.5, reserved_quantity: 2.5, delivered_quantity: 0, returned_quantity: 0, deficit_quantity: 0 }],
      customer_ids: [8], principal_user_id: "user-a", worker_ids: [], status: "registered",
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    let movementBody: unknown;
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [execution], total: 1, limit: 25, offset: 0 })),
      http.get(api("/locations"), () => HttpResponse.json({ data: [{ id: 11, name: "Bodega", kind: "storage", parent_id: null }] })),
      http.post(api("/executions/3/materials/item-1/deliver"), async ({ request }) => {
        movementBody = await request.json();
        return HttpResponse.json({ ...execution, materials: [{ ...execution.materials[0], reserved_quantity: 1.5, delivered_quantity: 1 }] });
      }),
    );
    renderApp("/ejecuciones");
    await screen.findByText("Requerido: 2.5 · Reservado: 2.5 · Entregado: 0 · Faltante: 0");
    await user.type(screen.getByLabelText("Cantidad"), "1");
    await user.selectOptions(screen.getByLabelText("Ubicación física"), "11");
    await user.click(screen.getByRole("button", { name: "Entregar" }));
    await screen.findByText("Material entregado al proyecto.");
    await waitFor(() => expect(movementBody).toMatchObject({ request_id: expect.stringMatching(/^[0-9a-f-]{36}$/), quantity: 1, location_id: 11, note: "" }));
  });

  it("shows the automatically generated shortage request on an execution", async () => {
    useAuthenticatedApi("admin");
    const execution = {
      id: 27, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, units: 1, estimated_days: 12,
      materials: [{ item_id: "item-1", required_quantity: 5, reserved_quantity: 2, delivered_quantity: 0, returned_quantity: 0, deficit_quantity: 3, purchased_pending_quantity: 1.5, unpurchased_quantity: 1.5 }],
      shortage_request_id: 31, customer_ids: [8], principal_user_id: "user-a", worker_ids: [], status: "completed",
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    server.use(http.get(api("/executions"), () => HttpResponse.json({ data: [execution], total: 1, limit: 25, offset: 0 })));
    renderApp("/ejecuciones");
    expect(await screen.findByText("Solicitud de insumos #31")).toBeInTheDocument();
    expect(screen.getByText("Sin comprar: 1.5 · Comprado pendiente de recibir: 1.5")).toBeInTheDocument();
  });

  it("links an ordered purchase line to the execution shortage", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    const execution = {
      id: 27, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, units: 1, estimated_days: 12,
      materials: [{ item_id: "item-1", required_quantity: 1.5, reserved_quantity: 0, delivered_quantity: 0, returned_quantity: 0, deficit_quantity: 1.5, purchased_pending_quantity: 0, unpurchased_quantity: 1.5 }],
      shortage_request_id: 31, customer_ids: [8], principal_user_id: "user-a", worker_ids: [], status: "registered",
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    const order = {
      id: 7, supplier_id: 3, supplier_name: "Proveedor Acero", status: "ordered", currency: "USD",
      exchange_rate: null, expected_at: null, notes: "", total_minor: 150, receipts: [],
      lines: [{ id: 42, item_id: "item-1", new_item: null, description: "Lámina", quantity: 1.5, received_qty: 0, unit_cost_minor: 100 }],
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z", ordered_at: "2026-01-01T00:00:00Z", cancelled_at: null,
    };
    let submitted: unknown;
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [execution], total: 1, limit: 25, offset: 0 })),
      http.get(api("/purchase-orders"), () => HttpResponse.json({ data: [order], total: 1, limit: 25, offset: 0 })),
      http.post(api("/executions/27/purchase-lines"), async ({ request }) => {
        submitted = await request.json();
        return HttpResponse.json({ ...execution, materials: [{ ...execution.materials[0], purchased_pending_quantity: 1.5, unpurchased_quantity: 0 }] });
      }),
    );
    renderApp("/ejecuciones");
    await screen.findByRole("option", { name: /Orden #7/ });
    await user.selectOptions(screen.getByLabelText("Línea de compra"), "42");
    await user.click(screen.getByRole("button", { name: "Vincular compra" }));
    await waitFor(() => expect(submitted).toEqual({ purchase_order_line_id: 42, quantity: 1.5 }));
  });

  it("records a partial production batch with a stable retry key and shows serial codes", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    let produced = 1;
    const execution = {
      id: 27, project_id: 4, project_name: "Marmita", description: "Acero inoxidable", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, units: 3, produced_units: produced, estimated_days: 12, materials: [],
      customer_ids: [8], principal_user_id: "user-a", worker_ids: [], status: "in_progress",
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    const firstUnit = { id: 1, code: "PRD-000001", execution_id: 27, produced_at: "2026-01-01T00:00:00Z" };
    const secondUnit = { id: 2, code: "PRD-000002", execution_id: 27, produced_at: "2026-01-02T00:00:00Z" };
    const requestIDs: string[] = [];
    let attempts = 0;
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [{ ...execution, produced_units: produced }], total: 1, limit: 25, offset: 0 })),
      http.get(api("/items"), () => HttpResponse.json(itemList())),
      http.get(api("/executions/27/units"), () => HttpResponse.json({ data: produced === 1 ? [firstUnit] : [firstUnit, secondUnit] })),
      http.post(api("/executions/27/produce"), async ({ request }) => {
        const body = await request.json() as { request_id: string; quantity: number };
        requestIDs.push(body.request_id);
        expect(body.quantity).toBe(1);
        attempts++;
        if (attempts === 1) return HttpResponse.error();
        produced = 2;
        return HttpResponse.json({ data: [secondUnit] }, { status: 201 });
      }),
    );
    renderApp("/ejecuciones");
    await user.click(await screen.findByText("Ver productos terminados"));
    expect(await screen.findByText("PRD-000001")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Unidades terminadas"), "1");
    await user.click(screen.getByRole("button", { name: "Registrar producción" }));
    await waitFor(() => expect(attempts).toBe(1));
    await user.click(screen.getByRole("button", { name: "Registrar producción" }));
    expect(await screen.findByText("PRD-000002")).toBeInTheDocument();
    expect(await screen.findByText("Producidas: 2 de 3")).toBeInTheDocument();
    expect(requestIDs).toHaveLength(2);
    expect(requestIDs[0]).toBe(requestIDs[1]);
  });

  it("hides production controls from a read-only execution user", async () => {
    useAuthenticatedApi("admin");
    const readOnly = session("admin");
    readOnly.user.permissions = ["executions:read"];
    server.use(
      http.post(api("/auth/refresh"), () => HttpResponse.json(readOnly)),
      http.get(api("/executions"), () => HttpResponse.json({ data: [{
        id: 27, project_id: 4, project_name: "Marmita", description: "", characteristics: [], consumables: [], tools: [],
        template_estimated_days: 12, units: 3, produced_units: 1, estimated_days: 12, materials: [], customer_ids: [8],
        principal_user_id: "user-a", worker_ids: [], status: "in_progress", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
      }], total: 1, limit: 25, offset: 0 })),
    );
    renderApp("/ejecuciones");
    expect(await screen.findByText("Producidas: 1 de 3")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Registrar producción" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Unidades terminadas")).not.toBeInTheDocument();
  });

  it("finishes only fully produced work and can cancel an open execution", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    const base = {
      project_id: 4, project_name: "Marmita", description: "", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, estimated_days: 12, materials: [], customer_ids: [8], principal_user_id: "user-a", worker_ids: [],
      created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    let completed = 0;
    let cancelled = 0;
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [
        { ...base, id: 27, units: 2, produced_units: 2, status: completed ? "completed" : "paused" },
        { ...base, id: 28, units: 1, produced_units: 0, status: cancelled ? "cancelled" : "registered" },
      ], total: 2, limit: 25, offset: 0 })),
      http.get(api("/items"), () => HttpResponse.json(itemList())),
      http.post(api("/executions/27/complete"), () => { completed++; return HttpResponse.json({ ...base, id: 27, units: 2, produced_units: 2, status: "completed" }); }),
      http.post(api("/executions/28/cancel"), () => { cancelled++; return HttpResponse.json({ ...base, id: 28, units: 1, produced_units: 0, status: "cancelled" }); }),
    );
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    try {
      renderApp("/ejecuciones");
      await user.click(await screen.findByRole("button", { name: "Finalizar fabricación" }));
      await waitFor(() => expect(completed).toBe(1));
      const cancelButtons = screen.getAllByRole("button", { name: "Cancelar ejecución" });
      await user.click(cancelButtons[0]);
      await waitFor(() => expect(cancelled).toBe(1));
      expect(confirm).toHaveBeenCalled();
    } finally {
      confirm.mockRestore();
    }
  });

  it("assigns finished units to a client at delivery and prints a numbered receipt", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    const execution = {
      id: 27, project_id: 4, project_name: "Marmita", description: "", characteristics: [], consumables: [], tools: [],
      template_estimated_days: 12, estimated_days: 12, units: 2, produced_units: 2, materials: [], customer_ids: [8, 9],
      principal_user_id: "user-a", worker_ids: [], status: "completed", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
    };
    const firstUnit = { id: 1, code: "PRD-000001", execution_id: 27, produced_at: "2026-01-01T00:00:00Z" };
    const secondUnit = { id: 2, code: "PRD-000002", execution_id: 27, produced_at: "2026-01-01T00:00:00Z" };
    const receipt = { id: 1, number: "DEL-000001", execution_id: 27, customer_id: 9, customer_name: "Cliente Sur", actor_id: "user-a", unit_codes: [firstUnit.code], delivered_at: "2026-01-02T12:00:00Z" };
    let delivered = false;
    let attempts = 0;
    const requestIDs: string[] = [];
    server.use(
      http.get(api("/executions"), () => HttpResponse.json({ data: [execution], total: 1, limit: 25, offset: 0 })),
      http.get(api("/executions/27/units"), () => HttpResponse.json({ data: delivered ? [{ ...firstUnit, delivery_id: 1, customer_id: 9, delivered_at: receipt.delivered_at }, secondUnit] : [firstUnit, secondUnit] })),
      http.get(api("/executions/27/deliveries"), () => HttpResponse.json({ data: delivered ? [receipt] : [] })),
      http.get(api("/customers"), () => HttpResponse.json({ data: customers, total: 2, limit: 100, offset: 0 })),
      http.post(api("/executions/27/deliveries"), async ({ request }) => {
        const body = await request.json() as { request_id: string; customer_id: number; unit_ids: number[] };
        requestIDs.push(body.request_id);
        expect(body.customer_id).toBe(9);
        expect(body.unit_ids).toEqual([1]);
        attempts++;
        if (attempts === 1) return HttpResponse.error();
        delivered = true;
        return HttpResponse.json(receipt, { status: 201 });
      }),
    );
    const print = vi.spyOn(window, "print").mockImplementation(() => {});
    try {
      renderApp("/ejecuciones");
      await user.click(await screen.findByText("Ver productos terminados"));
      await user.click(await screen.findByLabelText("Seleccionar PRD-000001"));
      await user.selectOptions(screen.getByLabelText("Cliente de entrega"), "9");
      await user.click(screen.getByRole("button", { name: "Entregar productos" }));
      await waitFor(() => expect(attempts).toBe(1));
      await user.click(screen.getByRole("button", { name: "Entregar productos" }));
      await screen.findByRole("button", { name: "Ver comprobante DEL-000001" });
      expect(requestIDs).toHaveLength(2);
      expect(requestIDs[0]).toBe(requestIDs[1]);
      await user.click(screen.getByRole("button", { name: "Ver comprobante DEL-000001" }));
      await user.click(screen.getByRole("button", { name: "Imprimir comprobante" }));
      expect(print).toHaveBeenCalledOnce();
      expect(screen.getAllByText("PRD-000001").length).toBeGreaterThan(0);
    } finally {
      print.mockRestore();
    }
  });
});
