import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { consumable, item } from "../test/fixtures";
import { api, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const asset = item();
const supply = consumable();

afterEach(() => vi.restoreAllMocks());

describe("item label sheet", () => {
  it("shows the QR that encodes the resolver URL, plus code and description", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api(`/items/${asset.id}`), () => HttpResponse.json(asset)));
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Etiqueta QR" }));

    const dialog = await screen.findByRole("dialog", { name: "Etiqueta QR" });
    const qr = within(dialog).getByTitle("Código QR de ISK-HER-001").closest("svg");
    expect(qr).toHaveAttribute("data-value", `${window.location.origin}/materiales/codigo/ISK-HER-001`);
    expect(within(dialog).getByText("ISK-HER-001")).toBeInTheDocument();
    expect(within(dialog).getByText("Taladro inalámbrico")).toBeInTheDocument();
    // The same label is rendered for paper.
    expect(document.querySelector(".print-root .label-card")).toHaveTextContent("ISK-HER-001");
  });

  it("prints and copies the link when native share is unavailable", async () => {
    useAuthenticatedApi("viewer");
    server.use(http.get(api(`/items/${asset.id}`), () => HttpResponse.json(asset)));
    const print = vi.spyOn(window, "print").mockImplementation(() => undefined);
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Etiqueta QR" }));
    await user.click(await screen.findByRole("button", { name: "Imprimir" }));
    expect(print).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: "Compartir" }));
    expect(await screen.findByText("Enlace copiado.")).toBeInTheDocument();
    expect(await navigator.clipboard.readText()).toBe(`${window.location.origin}/materiales/codigo/ISK-HER-001`);
  });
});

describe("labels page", () => {
  const serveItems = () =>
    server.use(
      http.get(api("/items"), () => HttpResponse.json({ data: [asset, supply], total: 2, limit: 25, offset: 0 })),
    );

  it("selects several items and prints one label per item", async () => {
    useAuthenticatedApi("operator");
    serveItems();
    const print = vi.spyOn(window, "print").mockImplementation(() => undefined);
    const user = userEvent.setup();
    renderApp("/materiales/etiquetas");
    expect(await screen.findByRole("heading", { name: "Etiquetas QR" })).toBeInTheDocument();
    const printButton = await screen.findByRole("button", { name: /Imprimir/ });
    expect(printButton).toBeDisabled();

    await user.click(await screen.findByRole("checkbox", { name: "Seleccionar ISK-HER-001" }));
    await user.click(screen.getByRole("checkbox", { name: "Seleccionar ISK-CON-001" }));
    expect(screen.getByText("2 seleccionados")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Imprimir (2)" }));
    expect(print).toHaveBeenCalled();
    await waitFor(() => expect(document.querySelectorAll(".print-root .label-card")).toHaveLength(2));
  });

  it("selects every visible item at once", async () => {
    useAuthenticatedApi("operator");
    serveItems();
    const user = userEvent.setup();
    renderApp("/materiales/etiquetas");
    await user.click(await screen.findByRole("button", { name: "Seleccionar visibles" }));
    expect(screen.getByText("2 seleccionados")).toBeInTheDocument();
  });

  it("is not available to viewers", async () => {
    useAuthenticatedApi("viewer");
    renderApp("/materiales/etiquetas");
    expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Imprimir/ })).not.toBeInTheDocument();
  });
});
