import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import type { Item } from "../lib/api/schemas";
import { item, maintenanceRecord, page } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const serve = (value: Item) => server.use(http.get(api(`/items/${value.id}`), () => HttpResponse.json(value)));
const serveOpenFailure = (record = maintenanceRecord()) =>
  server.use(http.get(api("/maintenance"), () => HttpResponse.json(page([record], { limit: 1 }))));

describe("report failure", () => {
  it("reports a failure with destination and status", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    let body: Record<string, unknown> = {};
    server.use(
      http.post(api(`/items/${asset.id}/maintenance`), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(maintenanceRecord({ item_id: asset.id }), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Reportar falla" }));
    const dialog = await screen.findByRole("dialog", { name: "Reportar falla" });
    await user.type(within(dialog).getByLabelText(/¿Qué falló\?/), "No enciende");
    await user.click(within(dialog).getByLabelText("En reparación"));
    await user.type(within(dialog).getByLabelText("Destino"), "Taller interno");
    await user.click(within(dialog).getByRole("button", { name: "Reportar falla" }));

    expect(await screen.findByText("Falla reportada en ISK-HER-001.")).toBeInTheDocument();
    expect(body).toMatchObject({ kind: "corrective", description: "No enciende", item_status: "in_repair", destination: "Taller interno" });
    expect(new Date(body.reported_at as string).getTime()).toBeLessThanOrEqual(Date.now());
  });

  it("validates the description and maps 422 and open_failure_exists", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    let call = 0;
    server.use(
      http.post(api(`/items/${asset.id}/maintenance`), () => {
        call += 1;
        return call === 1
          ? HttpResponse.json(errorBody("validation_failed", "x", { destination: "Destino demasiado largo" }), { status: 422 })
          : HttpResponse.json(errorBody("open_failure_exists", "x"), { status: 409 });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Reportar falla" }));
    const dialog = await screen.findByRole("dialog", { name: "Reportar falla" });
    await user.click(within(dialog).getByRole("button", { name: "Reportar falla" }));
    expect(await within(dialog).findByText("Describe la falla.")).toBeInTheDocument();
    expect(call).toBe(0);
    await user.type(within(dialog).getByLabelText(/¿Qué falló\?/), "Se rompió");
    await user.click(within(dialog).getByRole("button", { name: "Reportar falla" }));
    expect(await within(dialog).findByText("Destino demasiado largo")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Reportar falla" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("ya tiene una falla abierta");
  });

  it("offers update and close instead of a new report while a failure is open", async () => {
    useAuthenticatedApi("operator");
    const asset = item({ status: "inoperative", availability: "unavailable" });
    serve(asset);
    serveOpenFailure();
    renderApp(`/materiales/${asset.id}`);
    expect(await screen.findByRole("button", { name: "Cerrar falla" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Actualizar reparación" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Reportar falla" })).not.toBeInTheDocument();
  });
});

describe("update and close failure", () => {
  it("sends only the changed fields when updating the repair", async () => {
    useAuthenticatedApi("operator");
    const asset = item({ status: "inoperative", availability: "unavailable" });
    serve(asset);
    serveOpenFailure();
    let body: unknown;
    server.use(
      http.patch(api("/maintenance/1"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(maintenanceRecord({ item_status: "in_repair" }));
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Actualizar reparación" }));
    const dialog = await screen.findByRole("dialog", { name: "Actualizar reparación" });
    await user.click(within(dialog).getByLabelText("En reparación"));
    await user.type(within(dialog).getByLabelText("Destino"), "Espera de repuestos");
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await screen.findByText("Reparación actualizada.")).toBeInTheDocument();
    expect(body).toEqual({ destination: "Espera de repuestos", item_status: "in_repair" });
  });

  it("explains a closed record on update", async () => {
    useAuthenticatedApi("operator");
    const asset = item({ status: "inoperative", availability: "unavailable" });
    serve(asset);
    serveOpenFailure();
    server.use(http.patch(api("/maintenance/1"), () => HttpResponse.json(errorBody("record_closed", "x"), { status: 409 })));
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Actualizar reparación" }));
    const dialog = await screen.findByRole("dialog", { name: "Actualizar reparación" });
    await user.type(within(dialog).getByLabelText("Destino"), "X");
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("ya está cerrado");
  });

  it("closes the failure with cost and resolution", async () => {
    useAuthenticatedApi("operator");
    const asset = item({ status: "inoperative", availability: "unavailable" });
    serve(asset);
    serveOpenFailure();
    let body: unknown;
    server.use(
      http.post(api("/maintenance/1/close"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(maintenanceRecord({ status: "closed", item_status: "functional" }));
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Cerrar falla" }));
    const dialog = await screen.findByRole("dialog", { name: "Cerrar falla" });
    await user.type(within(dialog).getByLabelText("Costo"), "150.5");
    await user.type(within(dialog).getByLabelText("Nota de resolución"), "Cambio de batería");
    await user.click(within(dialog).getByRole("button", { name: "Cerrar falla" }));
    expect(await screen.findByText("Falla de ISK-HER-001 cerrada.")).toBeInTheDocument();
    expect(body).toEqual({ item_status_after: "functional", cost: 150.5, resolution_note: "Cambio de batería" });
  });

  it("rejects an invalid cost on the client", async () => {
    useAuthenticatedApi("operator");
    const asset = item({ status: "inoperative", availability: "unavailable" });
    serve(asset);
    serveOpenFailure();
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Cerrar falla" }));
    const dialog = await screen.findByRole("dialog", { name: "Cerrar falla" });
    await user.type(within(dialog).getByLabelText("Costo"), "abc");
    await user.click(within(dialog).getByRole("button", { name: "Cerrar falla" }));
    expect(await within(dialog).findByText(/monto válido/)).toBeInTheDocument();
  });
});

describe("preventive maintenance", () => {
  it("registers a preventive service with a next due date", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    let body: Record<string, unknown> = {};
    server.use(
      http.post(api(`/items/${asset.id}/maintenance`), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(maintenanceRecord({ kind: "preventive", status: "closed" }), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Registrar mantenimiento" }));
    const dialog = await screen.findByRole("dialog", { name: "Registrar mantenimiento" });
    await user.type(within(dialog).getByLabelText(/¿Qué se hizo\?/), "Lubricación");
    await user.type(within(dialog).getByLabelText("Costo"), "20");
    await user.type(within(dialog).getByLabelText("Próximo mantenimiento"), "2099-03-01");
    await user.click(within(dialog).getByRole("button", { name: "Registrar mantenimiento" }));
    expect(await screen.findByText("Mantenimiento registrado en ISK-HER-001.")).toBeInTheDocument();
    expect(body).toMatchObject({ kind: "preventive", description: "Lubricación", cost: 20 });
    expect(typeof body.next_due_at).toBe("string");
    expect(typeof body.performed_at).toBe("string");
  });

  it("does not offer maintenance flows to viewers", async () => {
    useAuthenticatedApi("viewer");
    const asset = item();
    serve(asset);
    renderApp(`/materiales/${asset.id}`);
    await screen.findByRole("heading", { name: "ISK-HER-001" });
    expect(screen.queryByRole("button", { name: "Reportar falla" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Registrar mantenimiento" })).not.toBeInTheDocument();
  });
});
