import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { AppProviders, createQueryClient } from "../providers";
import { session } from "../test/fixtures";
import { api } from "../test/api";
import { server } from "../test/server";
import { CustomersPage } from "./CustomersPage";
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
  return render(<AppProviders client={client}><MemoryRouter><CustomersPage /></MemoryRouter></AppProviders>);
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
    const dialog = screen.getByRole("dialog", {
      name: "Nuevo cliente"
    });
    await user.type(within(dialog).getByLabelText(/Nombre/), "Metal SA");
    await user.type(within(dialog).getByLabelText("Identificación"), "J456");
    await user.type(within(dialog).getByLabelText("Contacto"), "Luis");
    await user.click(within(dialog).getByRole("button", {
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
