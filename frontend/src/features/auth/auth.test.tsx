import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { session, summary } from "../../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../../test/render";
import { server } from "../../test/server";

const anonymous = () =>
  server.use(http.post(api("/auth/refresh"), () => HttpResponse.json(errorBody("unauthorized", "no session"), { status: 401 })));

describe("authentication flow", () => {
  it("sends anonymous visitors to the login page", async () => {
    anonymous();
    renderApp("/materiales");
    expect(await screen.findByRole("heading", { name: "Inventario" })).toBeInTheDocument();
    expect(screen.getByLabelText("Correo")).toBeInTheDocument();
  });

  it("restores the session from the refresh cookie on load", async () => {
    useAuthenticatedApi("operator");
    server.use(http.get(api("/dashboard/summary"), () => HttpResponse.json(summary)));
    renderApp("/");
    expect(await screen.findByRole("heading", { name: "Inicio" })).toBeInTheDocument();
    expect(await screen.findByText("48")).toBeInTheDocument();
  });

  it("logs in and lands on the dashboard", async () => {
    anonymous();
    server.use(
      http.post(api("/auth/login"), async ({ request }) => {
        const body = (await request.json()) as { email: string; password: string };
        return body.password === "correct-horse-1" ? HttpResponse.json(session("operator")) : HttpResponse.json(errorBody("invalid_credentials", "bad"), { status: 401 });
      }),
      http.get(api("/dashboard/summary"), () => HttpResponse.json(summary)),
    );
    const user = userEvent.setup();
    renderApp("/");
    await user.type(await screen.findByLabelText("Correo"), "ana@example.com");
    await user.type(screen.getByLabelText("Contraseña"), "correct-horse-1");
    await user.click(screen.getByRole("button", { name: "Entrar" }));
    expect(await screen.findByRole("heading", { name: "Inicio" })).toBeInTheDocument();
  });

  it("shows a readable message when credentials are wrong", async () => {
    anonymous();
    server.use(http.post(api("/auth/login"), () => HttpResponse.json(errorBody("invalid_credentials", "bad"), { status: 401 })));
    const user = userEvent.setup();
    renderApp("/login");
    await user.type(await screen.findByLabelText("Correo"), "ana@example.com");
    await user.type(screen.getByLabelText("Contraseña"), "wrong");
    await user.click(screen.getByRole("button", { name: "Entrar" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Correo o contraseña incorrectos.");
    expect(screen.getByRole("heading", { name: "Inventario" })).toBeInTheDocument();
  });

  it("validates the login form before calling the API", async () => {
    anonymous();
    const user = userEvent.setup();
    renderApp("/login");
    await user.click(await screen.findByRole("button", { name: "Entrar" }));
    expect(await screen.findByText("Escribe tu correo.")).toBeInTheDocument();
    expect(screen.getByText("Escribe tu contraseña.")).toBeInTheDocument();
  });

  it("refreshes on 401 and retries the failed request without bothering the user", async () => {
    useAuthenticatedApi("operator");
    let first = true;
    server.use(
      http.get(api("/dashboard/summary"), () => {
        if (first) {
          first = false;
          return HttpResponse.json(errorBody("unauthorized", "expired"), { status: 401 });
        }
        return HttpResponse.json(summary);
      }),
    );
    renderApp("/inicio");
    expect(await screen.findByText("48")).toBeInTheDocument();
  });

  it("goes back to login when the session cannot be refreshed", async () => {
    let refreshes = 0;
    server.use(
      http.post(api("/auth/refresh"), () => {
        refreshes += 1;
        return refreshes === 1 ? HttpResponse.json(session("operator")) : HttpResponse.json(errorBody("unauthorized", "revoked"), { status: 401 });
      }),
      http.get(api("/categories"), () => HttpResponse.json({ data: [] })),
      http.get(api("/locations"), () => HttpResponse.json({ data: [] })),
      http.get(api("/dashboard/summary"), () => HttpResponse.json(errorBody("unauthorized", "expired"), { status: 401 })),
    );
    renderApp("/inicio");
    expect(await screen.findByLabelText("Correo")).toBeInTheDocument();
  });

  it("logs out from the More page", async () => {
    useAuthenticatedApi("viewer");
    let loggedOut = false;
    server.use(
      http.post(api("/auth/logout"), () => {
        loggedOut = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    const user = userEvent.setup();
    renderApp("/mas");
    expect(await screen.findByText("Solo lectura")).toBeInTheDocument();
    await user.click(within(screen.getByRole("region", { name: "Cuenta" })).getByRole("button", { name: /cerrar sesión/i }));
    expect(await screen.findByLabelText("Correo")).toBeInTheDocument();
    expect(loggedOut).toBe(true);
    await waitFor(() => expect(screen.queryByText("Ana Pérez")).not.toBeInTheDocument());
  });
});

describe("offline start", () => {
  it("keeps the session and offers a retry when the server cannot be reached", async () => {
    let online = false;
    server.use(
      http.post(api("/auth/refresh"), () => (online ? HttpResponse.json(session("operator")) : HttpResponse.error())),
      http.get(api("/categories"), () => HttpResponse.json({ data: [] })),
      http.get(api("/locations"), () => HttpResponse.json({ data: [] })),
      http.get(api("/dashboard/summary"), () => HttpResponse.json(summary)),
    );
    const user = userEvent.setup();
    renderApp("/inicio");
    expect(await screen.findByText(/No se pudo conectar para verificar tu sesión/)).toBeInTheDocument();
    expect(screen.queryByLabelText("Correo")).not.toBeInTheDocument();

    online = true;
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByRole("heading", { name: "Inicio" })).toBeInTheDocument();
  });
});
