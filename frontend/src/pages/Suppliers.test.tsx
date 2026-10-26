import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { page, supplier } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

function serveSuppliers(rows = [supplier(), supplier({ id: 2, name: "Pinturas Norte", email: "", active: false })]) {
  server.use(http.get(api("/suppliers"), () => HttpResponse.json(page(rows))));
}

describe("suppliers page", () => {
  it("lists suppliers with their state and offers writes to operators", async () => {
    useAuthenticatedApi("operator");
    serveSuppliers();
    renderApp("/proveedores");
    const list = await screen.findByRole("list", { name: "Proveedores" });
    expect(within(list).getByText("Ferretería Sur")).toBeInTheDocument();
    expect(within(list).getByText("Inactivo")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Nuevo/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar Ferretería Sur" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Desactivar Ferretería Sur" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Activar Pinturas Norte" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Eliminar Ferretería Sur" })).toBeInTheDocument();
  });

  it("shows viewers a read-only list", async () => {
    useAuthenticatedApi("viewer");
    serveSuppliers();
    renderApp("/proveedores");
    await screen.findByText("Ferretería Sur");
    expect(screen.queryByRole("button", { name: /Nuevo/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Editar|Eliminar|Desactivar|Activar/ })).not.toBeInTheDocument();
  });

  it("sends the search text to the API and keeps it in the URL", async () => {
    useAuthenticatedApi("operator");
    const queries: string[] = [];
    server.use(
      http.get(api("/suppliers"), ({ request }) => {
        queries.push(new URL(request.url).searchParams.get("q") ?? "");
        return HttpResponse.json(page([supplier()]));
      }),
    );
    const user = userEvent.setup();
    renderApp("/proveedores");
    await screen.findByText("Ferretería Sur");
    await user.type(screen.getByLabelText("Buscar proveedores"), "sur");
    await screen.findByText("Ferretería Sur");
    await waitFor(() => expect(queries).toContain("sur"));
  });

  it("creates a supplier and confirms with a toast", async () => {
    useAuthenticatedApi("operator");
    serveSuppliers([]);
    let body: unknown;
    server.use(
      http.post(api("/suppliers"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(supplier({ id: 3, name: "Eléctrica Centro" }), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/proveedores");
    await user.click(await screen.findByRole("button", { name: /Nuevo/ }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo proveedor" });
    await user.type(within(dialog).getByLabelText(/Nombre/), "Eléctrica Centro");
    await user.type(within(dialog).getByLabelText("Correo"), "info@centro.example.com");
    await user.click(within(dialog).getByRole("button", { name: "Crear proveedor" }));
    expect(await screen.findByText("Proveedor Eléctrica Centro creado.")).toBeInTheDocument();
    expect(body).toEqual({ name: "Eléctrica Centro", email: "info@centro.example.com", active: true });
  });

  it("puts a duplicate name on the name input", async () => {
    useAuthenticatedApi("operator");
    serveSuppliers([]);
    server.use(
      http.post(api("/suppliers"), () => HttpResponse.json(errorBody("supplier_name_taken", "name taken"), { status: 409 })),
    );
    const user = userEvent.setup();
    renderApp("/proveedores");
    await user.click(await screen.findByRole("button", { name: /Nuevo/ }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo proveedor" });
    await user.type(within(dialog).getByLabelText(/Nombre/), "Ferretería Sur");
    await user.click(within(dialog).getByRole("button", { name: "Crear proveedor" }));
    expect(await within(dialog).findByText("Ya existe un proveedor con ese nombre.")).toBeInTheDocument();
    expect(within(dialog).getByLabelText(/Nombre/)).toHaveAccessibleDescription("Ya existe un proveedor con ese nombre.");
  });

  it("deactivates a supplier from the list", async () => {
    useAuthenticatedApi("operator");
    serveSuppliers();
    let body: unknown;
    server.use(
      http.patch(api("/suppliers/1"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(supplier({ active: false }));
      }),
    );
    const user = userEvent.setup();
    renderApp("/proveedores");
    await user.click(await screen.findByRole("button", { name: "Desactivar Ferretería Sur" }));
    expect(await screen.findByText("Proveedor Ferretería Sur desactivado.")).toBeInTheDocument();
    expect(body).toEqual({ active: false });
  });

  it("deletes a supplier after confirming", async () => {
    useAuthenticatedApi("operator");
    serveSuppliers();
    let deleted = false;
    server.use(
      http.delete(api("/suppliers/1"), () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/proveedores");
    await user.click(await screen.findByRole("button", { name: "Eliminar Ferretería Sur" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar proveedor" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar" }));
    expect(await screen.findByText("Proveedor Ferretería Sur eliminado.")).toBeInTheDocument();
    expect(deleted).toBe(true);
  });
});
