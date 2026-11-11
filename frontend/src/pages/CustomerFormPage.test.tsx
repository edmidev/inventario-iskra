import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { api, renderApp, useAuthenticatedApi } from "../test/render";
import { session } from "../test/fixtures";
import { server } from "../test/server";

const customer = { id: 1, name: "Acero SA", identification: "J123", contact: "Ana", notes: "Taller", active: true, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" };

describe("customer form pages", () => {
  it("opens creation on its own route and returns after saving", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    let saved: unknown;
    server.use(
      http.get(api("/customers"), () => HttpResponse.json({ data: [], total: 0, limit: 20, offset: 0 })),
      http.post(api("/customers"), async ({ request }) => { saved = await request.json(); return HttpResponse.json({ ...customer, ...(saved as object) }, { status: 201 }); }),
    );
    renderApp("/clientes/nuevo");
    expect(await screen.findByRole("heading", { name: "Nuevo cliente" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await user.type(screen.getByLabelText(/Nombre/), "Metal SA");
    await user.click(screen.getByRole("button", { name: "Crear cliente" }));
    expect(await screen.findByRole("heading", { name: "Clientes" })).toBeInTheDocument();
    expect(saved).toEqual({ name: "Metal SA", active: true });
  });

  it("loads a customer directly by ID and offers a deterministic return", async () => {
    useAuthenticatedApi("admin");
    server.use(http.get(api("/customers/1"), () => HttpResponse.json(customer)));
    renderApp("/clientes/1/editar");
    expect(await screen.findByDisplayValue("Acero SA")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Volver a clientes/ })).toHaveAttribute("href", "/clientes");
  });

  it("edits a customer and returns to the filtered list", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    let changes: unknown;
    server.use(
      http.get(api("/customers"), () => HttpResponse.json({ data: [customer], total: 1, limit: 20, offset: 0 })),
      http.get(api("/customers/1"), () => HttpResponse.json(customer)),
      http.patch(api("/customers/1"), async ({ request }) => { changes = await request.json(); return HttpResponse.json({ ...customer, ...(changes as object) }); }),
    );
    renderApp("/clientes?q=Acero");
    await user.click(await screen.findByRole("button", { name: "Editar Acero SA" }));
    const name = await screen.findByDisplayValue("Acero SA");
    await user.clear(name);
    await user.type(name, "Acero Norte");
    await user.click(screen.getByRole("button", { name: "Guardar cambios" }));
    expect(await screen.findByRole("heading", { name: "Clientes" })).toBeInTheDocument();
    expect(screen.getByLabelText("Buscar clientes")).toHaveValue("Acero");
    expect(changes).toEqual({ name: "Acero Norte" });
  });

  it("does not expose the form without write access", async () => {
    useAuthenticatedApi("admin");
    server.use(http.post(api("/auth/refresh"), () => HttpResponse.json({ ...session("worker"), user: { ...session("worker").user, permissions: ["customers:read"] } })));
    renderApp("/clientes/nuevo");
    expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Crear cliente" })).not.toBeInTheDocument();
  });

  it("does not fetch the customer on direct editing without write access", async () => {
    useAuthenticatedApi("admin");
    let reads = 0;
    server.use(
      http.post(api("/auth/refresh"), () => HttpResponse.json({ ...session("worker"), user: { ...session("worker").user, permissions: ["customers:read"] } })),
      http.get(api("/customers/1"), () => { reads++; return HttpResponse.json(customer); }),
    );
    renderApp("/clientes/1/editar");
    expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Guardar cambios" })).not.toBeInTheDocument();
    expect(reads).toBe(0);
  });

  it("shows an error and return link when the customer does not exist", async () => {
    useAuthenticatedApi("admin");
    server.use(http.get(api("/customers/1"), () => HttpResponse.json({ error: { code: "not_found", message: "not found" } }, { status: 404 })));
    renderApp("/clientes/1/editar");
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Volver a clientes/ })).toHaveAttribute("href", "/clientes");
    expect(screen.queryByRole("button", { name: "Guardar cambios" })).not.toBeInTheDocument();
  });

  it("cancels editing back to the filtered list without saving", async () => {
    const user = userEvent.setup();
    useAuthenticatedApi("admin");
    let writes = 0;
    server.use(
      http.get(api("/customers"), () => HttpResponse.json({ data: [customer], total: 1, limit: 20, offset: 0 })),
      http.get(api("/customers/1"), () => HttpResponse.json(customer)),
      http.patch(api("/customers/1"), () => { writes++; return HttpResponse.json(customer); }),
    );
    renderApp("/clientes?q=Acero");
    await user.click(await screen.findByRole("button", { name: "Editar Acero SA" }));
    await screen.findByDisplayValue("Acero SA");
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(await screen.findByLabelText("Buscar clientes")).toHaveValue("Acero");
    expect(writes).toBe(0);
  });
});
