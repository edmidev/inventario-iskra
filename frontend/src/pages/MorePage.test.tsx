import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, useAuthenticatedApi } from "../test/render";

describe("more page", () => {
  it("links to suppliers and purchase orders", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/mas");
    const nav = await screen.findByRole("navigation", { name: "Operaciones" });
    expect(within(nav).getByRole("link", { name: /Proveedores/ })).toHaveAttribute("href", "/proveedores");
    expect(within(nav).getByRole("link", { name: /Órdenes de compra/ })).toHaveAttribute("href", "/ordenes-compra");
  });
});
