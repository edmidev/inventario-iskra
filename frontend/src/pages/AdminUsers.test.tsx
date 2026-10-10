import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import type { User } from "../lib/api/schemas";
import { user as makeUser } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const admin: User = { ...makeUser("admin"), id: "u-1", name: "Ana Pérez", email: "ana@example.com" };
const people: User[] = [
  admin,
  { ...makeUser("operator"), id: "u-2", name: "Luis Mora", email: "luis@example.com" },
  { ...makeUser("viewer"), id: "u-3", name: "Eva Ríos", email: "eva@example.com", active: false },
];

const serveUsers = () => server.use(http.get(api("/users"), () => HttpResponse.json({ data: people })));

describe("admin access", () => {
  it("blocks non-admins from the admin routes", async () => {
    useAuthenticatedApi("operator");
    renderApp("/admin/usuarios");
    expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Usuarios" })).not.toBeInTheDocument();
  });

  it("hides the administration links from non-admins on the More page", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    expect(await screen.findByRole("heading", { name: "Más" })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Administración" })).not.toBeInTheDocument();
    expect(screen.queryByText("Usuarios")).not.toBeInTheDocument();
  });

  it("shows the administration links to admins", async () => {
    useAuthenticatedApi("admin");
    renderApp("/mas");
    const nav = await screen.findByRole("navigation", { name: "Administración" });
    expect(within(nav).getByRole("link", { name: /Usuarios/ })).toHaveAttribute("href", "/admin/usuarios");
    expect(within(nav).getByRole("link", { name: /Categorías/ })).toHaveAttribute("href", "/admin/categorias");
    expect(within(nav).getByRole("link", { name: /Ubicaciones/ })).toHaveAttribute("href", "/admin/ubicaciones");
  });
});

describe("users administration", () => {
  it("lists users and filters by search, role and state", async () => {
    useAuthenticatedApi("admin");
    serveUsers();
    const user = userEvent.setup();
    renderApp("/admin/usuarios");
    const list = await screen.findByRole("list", { name: "Usuarios" });
    expect(within(list).getAllByRole("listitem")).toHaveLength(3);
    expect(within(list).getByText("(tú)")).toBeInTheDocument();
    expect(within(list).getByText("Inactivo")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Buscar usuarios"), "luis");
    await waitFor(() => expect(within(screen.getByRole("list", { name: "Usuarios" })).getAllByRole("listitem")).toHaveLength(1));
    expect(screen.getByText("Luis Mora")).toBeInTheDocument();
  });

  it("creates a user and sends the trimmed values", async () => {
    useAuthenticatedApi("admin");
    serveUsers();
    let body: unknown;
    server.use(
      http.post(api("/users"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ ...people[1], id: "u-9", name: "Nora Díaz", email: "nora@example.com" }, { status: 201 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/usuarios");
    await user.click(await screen.findByRole("button", { name: "Nuevo" }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo usuario" });
    await user.type(within(dialog).getByLabelText(/Nombre/), "  Nora Díaz ");
    await user.type(within(dialog).getByLabelText(/Correo/), "nora@example.com");
    await user.type(within(dialog).getByLabelText(/Contraseña inicial/), "clave-segura-1");
    await user.click(within(dialog).getByRole("button", { name: "Crear usuario" }));
    expect(await screen.findByText("Usuario Nora Díaz creado.")).toBeInTheDocument();
    expect(body).toEqual({ name: "Nora Díaz", email: "nora@example.com", role: "operator", password: "clave-segura-1" });
  });

  it("validates before calling the API and maps a duplicated email to the field", async () => {
    useAuthenticatedApi("admin");
    serveUsers();
    server.use(http.post(api("/users"), () => HttpResponse.json(errorBody("email_taken", "email already in use"), { status: 409 })));
    const user = userEvent.setup();
    renderApp("/admin/usuarios");
    await user.click(await screen.findByRole("button", { name: "Nuevo" }));
    const dialog = await screen.findByRole("dialog", { name: "Nuevo usuario" });
    await user.click(within(dialog).getByRole("button", { name: "Crear usuario" }));
    expect(await within(dialog).findByText("Escribe el nombre.")).toBeInTheDocument();
    expect(within(dialog).getByText("Escribe el correo.")).toBeInTheDocument();
    expect(within(dialog).getByText("Usa al menos 10 caracteres.")).toBeInTheDocument();

    await user.type(within(dialog).getByLabelText(/Nombre/), "Eva");
    await user.type(within(dialog).getByLabelText(/Correo/), "eva@example.com");
    await user.type(within(dialog).getByLabelText(/Contraseña inicial/), "clave-segura-1");
    await user.click(within(dialog).getByRole("button", { name: "Crear usuario" }));
    expect(await within(dialog).findByText("Ya existe un usuario con ese correo.")).toBeInTheDocument();
  });

  it("explains the last-admin rule when demoting the only admin", async () => {
    useAuthenticatedApi("admin");
    serveUsers();
    server.use(http.patch(api("/users/u-1"), () => HttpResponse.json(errorBody("last_admin", "cannot deactivate or demote the last active admin"), { status: 409 })));
    const user = userEvent.setup();
    renderApp("/admin/usuarios");
    await user.click(await screen.findByRole("button", { name: "Editar Ana Pérez" }));
    const dialog = await screen.findByRole("dialog", { name: "Editar usuario" });
    await user.click(within(dialog).getByRole("checkbox", { name: /Cuenta activa/ }));
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent(
      "No puedes desactivar ni quitar el rol al último administrador activo.",
    );
  });

  it("sends only the changed fields when editing", async () => {
    useAuthenticatedApi("admin");
    serveUsers();
    let body: unknown;
    server.use(
      http.patch(api("/users/u-2"), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json({ ...people[1], name: "Luis M." });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/usuarios");
    await user.click(await screen.findByRole("button", { name: "Editar Luis Mora" }));
    const dialog = await screen.findByRole("dialog", { name: "Editar usuario" });
    const name = within(dialog).getByLabelText(/Nombre/);
    await user.clear(name);
    await user.type(name, "Luis M.");
    await user.click(within(dialog).getByRole("button", { name: "Guardar cambios" }));
    expect(await screen.findByText("Usuario actualizado.")).toBeInTheDocument();
    expect(body).toEqual({ name: "Luis M." });
  });

  it("resets a password and rejects short ones locally", async () => {
    useAuthenticatedApi("admin");
    serveUsers();
    let body: unknown;
    server.use(
      http.post(api("/users/u-2/password"), async ({ request }) => {
        body = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/admin/usuarios");
    await user.click(await screen.findByRole("button", { name: "Restablecer contraseña de Luis Mora" }));
    const dialog = await screen.findByRole("dialog", { name: "Restablecer contraseña" });
    await user.type(within(dialog).getByLabelText(/Nueva contraseña/), "corta");
    await user.click(within(dialog).getByRole("button", { name: "Restablecer" }));
    expect(await within(dialog).findByText("Usa al menos 10 caracteres.")).toBeInTheDocument();
    expect(body).toBeUndefined();
    await user.clear(within(dialog).getByLabelText(/Nueva contraseña/));
    await user.type(within(dialog).getByLabelText(/Nueva contraseña/), "nueva-clave-123");
    await user.click(within(dialog).getByRole("button", { name: "Restablecer" }));
    expect(await screen.findByText("Contraseña de Luis Mora restablecida.")).toBeInTheDocument();
    expect(body).toEqual({ new_password: "nueva-clave-123" });
  });
});

describe("my account", () => {
  const fill = async (user: ReturnType<typeof userEvent.setup>, current: string, next: string, again: string) => {
    await user.type(await screen.findByLabelText(/Contraseña actual/), current);
    await user.type(screen.getByLabelText(/^Nueva contraseña/), next);
    await user.type(screen.getByLabelText(/Repite/), again);
    await user.click(screen.getByRole("button", { name: "Cambiar contraseña" }));
  };

  it("changes the password and sends the user back to the login", async () => {
    useAuthenticatedApi("operator");
    let body: unknown;
    server.use(
      http.post(api("/auth/password"), async ({ request }) => {
        body = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
      http.post(api("/auth/logout"), () => new HttpResponse(null, { status: 204 })),
    );
    const user = userEvent.setup();
    renderApp("/cuenta");
    await fill(user, "vieja-clave-1", "nueva-clave-123", "nueva-clave-123");
    expect(await screen.findByLabelText("Correo")).toBeInTheDocument();
    expect(body).toEqual({ current_password: "vieja-clave-1", new_password: "nueva-clave-123" });
  });

  it("flags a mismatching confirmation without calling the API", async () => {
    useAuthenticatedApi("operator");
    const user = userEvent.setup();
    renderApp("/cuenta");
    await fill(user, "vieja-clave-1", "nueva-clave-123", "otra-clave-1234");
    expect(await screen.findByText("Las contraseñas no coinciden.")).toBeInTheDocument();
  });

  it("shows the wrong current password on its field", async () => {
    useAuthenticatedApi("operator");
    server.use(http.post(api("/auth/password"), () => HttpResponse.json(errorBody("invalid_current_password", "bad"), { status: 400 })));
    const user = userEvent.setup();
    renderApp("/cuenta");
    await fill(user, "equivocada-1", "nueva-clave-123", "nueva-clave-123");
    expect(await screen.findByText("La contraseña actual no es correcta.")).toBeInTheDocument();
  });

  it("maps 422 field errors from the API", async () => {
    useAuthenticatedApi("operator");
    server.use(
      http.post(api("/auth/password"), () =>
        HttpResponse.json(errorBody("validation_failed", "bad", { new_password: "La contraseña es demasiado común." }), { status: 422 }),
      ),
    );
    const user = userEvent.setup();
    renderApp("/cuenta");
    await fill(user, "vieja-clave-1", "password12345", "password12345");
    expect(await screen.findByText("La contraseña es demasiado común.")).toBeInTheDocument();
  });
});
