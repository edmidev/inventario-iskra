import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { categories } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

describe("categories administration", () => {
  it("lists categories with their prefix", async () => {
    useAuthenticatedApi("admin");
    renderApp("/admin/categorias");
    const list = await screen.findByRole("list", { name: "Categorías" });
    expect(within(list).getByText("Herramientas")).toBeInTheDocument();
    expect(within(list).getByText("HER")).toBeInTheDocument();
  });

  it("creates a category with an uppercased prefix", async () => {
    useAuthenticatedApi("admin");
    let body: unknown;
    server.use(
      http.post(api("/categories"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ id: 3, name: "Equipo de seguridad", code_prefix: "SEG" }, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/categorias");
    await user.click(await screen.findByRole("button", { name: "Nueva" }));
    const dialog = await screen.findByRole("dialog", { name: "Nueva categoría" });
    await user.type(within(dialog).getByLabelText(/Nombre/), "Equipo de seguridad");
    await user.type(within(dialog).getByLabelText(/Prefijo/), "seg");
    await user.click(within(dialog).getByRole("button", { name: "Crear categoría" }));
    expect(await screen.findByText("Categoría Equipo de seguridad creada.")).toBeInTheDocument();
    expect(body).toEqual({ name: "Equipo de seguridad", code_prefix: "SEG" });
  });

  it("validates the prefix locally", async () => {
    useAuthenticatedApi("admin");
    const user = userEvent.setup();
    renderApp("/admin/categorias");
    await user.click(await screen.findByRole("button", { name: "Nueva" }));
    const dialog = await screen.findByRole("dialog", { name: "Nueva categoría" });
    await user.type(within(dialog).getByLabelText(/Nombre/), "X");
    await user.type(within(dialog).getByLabelText(/Prefijo/), "a b");
    await user.click(within(dialog).getByRole("button", { name: "Crear categoría" }));
    expect(await within(dialog).findByText("Usa de 2 a 8 letras o números, sin espacios.")).toBeInTheDocument();
  });

  it("maps a taken prefix (409) and 422 field errors onto the inputs", async () => {
    useAuthenticatedApi("admin");
    let attempt = 0;
    server.use(
      http.post(api("/categories"), () => {
        attempt += 1;
        return attempt === 1
          ? HttpResponse.json(errorBody("category_prefix_taken", "taken"), { status: 409 })
          : HttpResponse.json(errorBody("validation_failed", "bad", { name: "El nombre ya existe." }), { status: 422 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/categorias");
    await user.click(await screen.findByRole("button", { name: "Nueva" }));
    const dialog = await screen.findByRole("dialog", { name: "Nueva categoría" });
    await user.type(within(dialog).getByLabelText(/Nombre/), "Otra");
    await user.type(within(dialog).getByLabelText(/Prefijo/), "HER");
    await user.click(within(dialog).getByRole("button", { name: "Crear categoría" }));
    expect(await within(dialog).findByText("Ese prefijo ya lo usa otra categoría.")).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Crear categoría" }));
    expect(await within(dialog).findByText("El nombre ya existe.")).toBeInTheDocument();
  });

  it("edits only what changed", async () => {
    useAuthenticatedApi("admin");
    let body: unknown;
    server.use(
      http.patch(api("/categories/1"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ ...categories[0], name: "Herramientas manuales" });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/categorias");
    await user.click(await screen.findByRole("button", { name: "Editar Herramientas" }));
    const dialog = await screen.findByRole("dialog", { name: "Editar categoría" });
    const name = within(dialog).getByLabelText(/Nombre/);
    await user.clear(name);
    await user.type(name, "Herramientas manuales");
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await screen.findByText("Categoría actualizada.")).toBeInTheDocument();
    expect(body).toMatchObject({ name: "Herramientas manuales" });
  });

  it("explains why a category in use cannot be deleted", async () => {
    useAuthenticatedApi("admin");
    server.use(http.delete(api("/categories/1"), () => HttpResponse.json(errorBody("category_in_use", "in use"), { status: 409 })));
    const user = userEvent.setup();
    renderApp("/admin/categorias");
    await user.click(await screen.findByRole("button", { name: "Eliminar Herramientas" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar categoría" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("La categoría todavía tiene materiales.");
  });

  it("deletes an unused category", async () => {
    useAuthenticatedApi("admin");
    let deleted = false;
    server.use(
      http.delete(api("/categories/2"), () => {
        deleted = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/categorias");
    await user.click(await screen.findByRole("button", { name: "Eliminar Consumibles" }));
    await user.click(within(await screen.findByRole("dialog", { name: "Eliminar categoría" })).getByRole("button", { name: "Eliminar" }));
    expect(await screen.findByText("Categoría eliminada.")).toBeInTheDocument();
    expect(deleted).toBe(true);
  });

  it("is hidden from operators", async () => {
    useAuthenticatedApi("operator");
    renderApp("/admin/categorias");
    expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
  });
});

describe("locations administration", () => {
  it("shows locations as an indented tree", async () => {
    useAuthenticatedApi("admin");
    renderApp("/admin/ubicaciones");
    const list = await screen.findByRole("list", { name: "Ubicaciones" });
    const rows = within(list).getAllByRole("listitem");
    expect(rows.map((r) => r.querySelector("p")?.textContent)).toEqual(["Bodega", "Estante A", "Juan Gómez"]);
    expect(rows[1]?.style.marginLeft).toBe("1.25rem");
    expect(within(rows[2] as HTMLElement).getByText("Persona")).toBeInTheDocument();
  });

  it("creates a location inside a parent", async () => {
    useAuthenticatedApi("admin");
    let body: unknown;
    server.use(
      http.post(api("/locations"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ id: 20, name: "Caja 1", kind: "container", parent_id: 11 }, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/ubicaciones");
    await user.click(await screen.findByRole("button", { name: "Nueva" }));
    const dialog = await screen.findByRole("dialog", { name: "Nueva ubicación" });
    await user.type(within(dialog).getByLabelText(/Nombre/), "Caja 1");
    await user.click(within(dialog).getByRole("combobox", { name: /Tipo/ }));
    await user.click(await screen.findByRole("option", { name: "Contenedor" }));
    await user.click(within(dialog).getByRole("combobox", { name: /Dentro de/ }));
    await user.click(await screen.findByRole("option", { name: "Bodega › Estante A" }));
    await user.click(within(dialog).getByRole("button", { name: "Crear ubicación" }));
    expect(await screen.findByText("Ubicación Caja 1 creada.")).toBeInTheDocument();
    expect(body).toEqual({ name: "Caja 1", kind: "container", parent_id: 11 });
  });

  it("does not offer a location or its descendants as its own parent", async () => {
    useAuthenticatedApi("admin");
    const user = userEvent.setup();
    renderApp("/admin/ubicaciones");
    await user.click(await screen.findByRole("button", { name: "Editar Bodega" }));
    const dialog = await screen.findByRole("dialog", { name: "Editar ubicación" });
    await user.click(within(dialog).getByRole("combobox", { name: /Dentro de/ }));
    const options = (await screen.findAllByRole("option")).map((o) => o.textContent);
    expect(options).toContain("Juan Gómez");
    expect(options).not.toContain("Bodega");
    expect(options).not.toContain("Bodega › Estante A");
  });

  it("shows the cycle conflict on the parent field", async () => {
    useAuthenticatedApi("admin");
    server.use(http.patch(api("/locations/12"), () => HttpResponse.json(errorBody("location_cycle", "cycle"), { status: 409 })));
    const user = userEvent.setup();
    renderApp("/admin/ubicaciones");
    await user.click(await screen.findByRole("button", { name: "Editar Juan Gómez" }));
    const dialog = await screen.findByRole("dialog", { name: "Editar ubicación" });
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await within(dialog).findByText("Esa ubicación padre crearía un ciclo.")).toBeInTheDocument();
  });

  it("explains when a location is still in use", async () => {
    useAuthenticatedApi("admin");
    server.use(http.delete(api("/locations/10"), () => HttpResponse.json(errorBody("location_in_use", "in use"), { status: 409 })));
    const user = userEvent.setup();
    renderApp("/admin/ubicaciones");
    await user.click(await screen.findByRole("button", { name: "Eliminar Bodega" }));
    const dialog = await screen.findByRole("dialog", { name: "Eliminar ubicación" });
    await user.click(within(dialog).getByRole("button", { name: "Eliminar" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("La ubicación todavía tiene materiales o sububicaciones.");
  });

  it("is hidden from viewers", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/admin/ubicaciones");
    expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
  });
});
