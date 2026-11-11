import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { api, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

describe("manufacturing routes", () => {
  it.each([["/clientes", "Clientes"], ["/proyectos", "Proyectos"]])("opens %s with explicit administrator grants", async (path, title) => {
    useAuthenticatedApi("admin");
    server.use(http.get(api(path === "/clientes" ? "/customers" : "/projects"), () => HttpResponse.json({data: [], total: 0, limit: 25, offset: 0})));
    renderApp(path);
    expect(await screen.findByRole("heading", {name: title})).toBeInTheDocument();
  });
  it("opens executions for a user with the execution read grant", async () => {
    useAuthenticatedApi("admin");
    server.use(http.get(api("/executions"), () => HttpResponse.json({data: [], total: 0, limit: 25, offset: 0})));
    renderApp("/ejecuciones");
    expect(await screen.findByRole("heading", {name: "Ejecuciones"})).toBeInTheDocument();
  });
  it.each(["/clientes", "/proyectos", "/proyectos/1", "/ejecuciones"])("denies %s without the dedicated capability", async (path) => {
    useAuthenticatedApi("viewer");
    renderApp(path);
    expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
  });
});
