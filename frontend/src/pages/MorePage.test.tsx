import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, useAuthenticatedApi } from "../test/render";

describe("more menu", () => {
  it("retains suppliers without duplicating purchase orders in operations", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    const nav = await screen.findByRole("navigation", { name: "Operaciones" });
    expect(within(nav).getByRole("link", { name: /Proveedores/ })).toHaveAttribute("href", "/proveedores");
    expect(within(nav).queryByRole("link", { name: /Órdenes de compra/ })).not.toBeInTheDocument();
  });

  it("preserves sidebar and bottom-bar navigation", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    await screen.findByRole("navigation", { name: "Operaciones" });
    const [sidebar, bottom] = screen.getAllByRole("navigation", { name: "Navegación principal" });
    const names = (nav: HTMLElement) => within(nav).getAllByRole("link").map((link) => link.textContent);
    expect(names(sidebar)).toEqual(["Inicio", "Materiales", "Escanear", "Proveedores", "Órdenes de compra", "Más"]);
    expect(names(bottom)).toEqual(["Inicio", "Materiales", "Escanear", "Órdenes de compra", "Más"]);
    expect(within(bottom).getByRole("link", { name: "Órdenes de compra" })).toHaveAttribute("href", "/ordenes-compra");
  });

  it("presents authorized destinations as touch-friendly icon tiles", async () => {
    useAuthenticatedApi("admin");
    renderApp("/mas");
    const manufacturing = await screen.findByRole("navigation", { name: "Fabricación" });
    expect(manufacturing).toHaveClass("grid");
    const projects = within(manufacturing).getByRole("link", { name: /Proyectos/ });
    expect(projects).toHaveAttribute("href", "/proyectos");
    expect(projects.querySelector("svg")).toBeInTheDocument();
    expect(within(manufacturing).getByRole("link", { name: /Productos terminados/ })).toHaveAttribute("href", "/productos-terminados");
    expect(screen.getByRole("navigation", { name: "Administración" })).toHaveClass("grid");
    expect(within(screen.getByRole("region", { name: "Cuenta" })).getByRole("button", { name: /Cerrar sesión/ })).toBeInTheDocument();
  });

  it("continues to hide destinations the user cannot access", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    expect(await screen.findByRole("navigation", { name: "Operaciones" })).toHaveClass("grid");
    expect(screen.queryByRole("navigation", { name: "Fabricación" })).not.toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Administración" })).not.toBeInTheDocument();
  });
});
