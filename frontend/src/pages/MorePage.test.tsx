import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, useAuthenticatedApi } from "../test/render";

describe("more page", () => {
  it("still links to suppliers, which are not in the mobile bar, and no longer repeats purchase orders", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    const nav = await screen.findByRole("navigation", { name: "Operaciones" });
    expect(within(nav).getByRole("link", { name: /Proveedores/ })).toHaveAttribute("href", "/proveedores");
    expect(within(nav).queryByRole("link", { name: /Órdenes de compra/ })).not.toBeInTheDocument();
  });
});

describe("main navigation", () => {
  it("shows suppliers and purchase orders in the sidebar but only purchase orders in the bottom bar", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    await screen.findByRole("navigation", { name: "Operaciones" });
    const [sidebar, bottom] = screen.getAllByRole("navigation", { name: "Navegación principal" });
    const names = (nav: HTMLElement) => within(nav).getAllByRole("link").map((link) => link.textContent);
    expect(names(sidebar)).toEqual(["Inicio", "Materiales", "Escanear", "Proveedores", "Órdenes de compra", "Más"]);
    expect(names(bottom)).toEqual(["Inicio", "Materiales", "Escanear", "Órdenes de compra", "Más"]);
    expect(within(bottom).getByRole("link", { name: "Órdenes de compra" })).toHaveAttribute("href", "/ordenes-compra");
  });
});
