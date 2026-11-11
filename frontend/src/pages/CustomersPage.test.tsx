import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppProviders, createQueryClient } from "../providers";
import { session } from "../test/fixtures";
import { api } from "../test/api";
import { server } from "../test/server";
import { CustomersPage } from "./CustomersPage";
import { CustomerFormPage } from "./CustomerFormPage";
const customer = {
  id: 1,
  name: "Acero SA",
  identification: "J123",
  contact: "Ana",
  notes: "Taller",
  active: true,
  created_at: "2026-11-05T00:00:00Z",
  updated_at: "2026-11-05T00:00:00Z"
};
function setup(grants: string[]) {
  server.use(http.post(api("/auth/refresh"), () => HttpResponse.json({
    ...session("worker"),
    user: {
      ...session("worker").user,
      permissions: grants
    }
  })), http.get(api("/customers"), () => HttpResponse.json({
    data: [customer],
    total: 1,
    limit: 25,
    offset: 0
  })));
  const client = createQueryClient();
  return render(<AppProviders client={client}><MemoryRouter initialEntries={["/clientes"]}><Routes><Route path="/clientes" element={<CustomersPage />} /><Route path="/clientes/nuevo" element={<CustomerFormPage />} /><Route path="/clientes/:id/editar" element={<CustomerFormPage />} /></Routes></MemoryRouter></AppProviders>);
}
describe("customers", () => {
  it("shows customer identity but hides writes without the explicit grant", async () => {
    setup(["customers:read"]);
    expect(await screen.findByText("Acero SA")).toBeInTheDocument();
    expect(screen.getByText("J123")).toBeInTheDocument();
    expect(screen.queryByRole("button", {
      name: "Nuevo"
    })).not.toBeInTheDocument();
  });
  it("creates a customer with identity and contact", async () => {
    let saved: unknown;
    server.use(http.post(api("/customers"), async ({
      request
    }) => {
      saved = await request.json();
      return HttpResponse.json({
        ...customer,
        ...(saved as object),
        id: 2
      }, {
        status: 201
      });
    }));
    setup(["customers:read", "customers:write"]);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", {
      name: "Nuevo"
    }));
    expect(await screen.findByRole("heading", { name: "Nuevo cliente" })).toBeInTheDocument();
    await user.type(screen.getByLabelText(/Nombre/), "Metal SA");
    await user.type(screen.getByLabelText("Identificación"), "J456");
    await user.type(screen.getByLabelText("Contacto"), "Luis");
    await user.click(screen.getByRole("button", {
      name: "Crear cliente"
    }));
    expect(await screen.findByText("Cliente Metal SA creado.")).toBeInTheDocument();
    expect(saved).toEqual({
      name: "Metal SA",
      identification: "J456",
      contact: "Luis",
      active: true
    });
  });
});
