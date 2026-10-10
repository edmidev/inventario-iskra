import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CameraError } from "../features/scan/scanner";
import { item } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const scanner = vi.hoisted(() => ({
  onCode: null as ((value: string) => void) | null,
  start: vi.fn<(video: HTMLVideoElement, onCode: (value: string) => void) => Promise<unknown>>(),
  stop: vi.fn<() => void>(),
}));

vi.mock("../features/scan/scanner", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../features/scan/scanner")>();
  return { ...actual, startScanner: scanner.start };
});

const asset = item();
const serveByCode = () =>
  server.use(
    http.get(api("/items/by-code/:code"), ({ params }) =>
      String(params.code).toUpperCase() === asset.code ? HttpResponse.json(asset) : HttpResponse.json(errorBody("not_found", "no"), { status: 404 }),
    ),
    http.get(api(`/items/${asset.id}`), () => HttpResponse.json(asset)),
  );

beforeEach(() => {
  scanner.onCode = null;
  scanner.stop.mockReset();
  scanner.start.mockReset();
  scanner.start.mockImplementation(async (_video, onCode) => {
    scanner.onCode = onCode;
    return { stop: scanner.stop, hasTorch: true, toggleTorch: async () => true };
  });
});

describe("scan page", () => {
  it("resolves a manually typed code and opens the item", async () => {
    useAuthenticatedApi("viewer");
    serveByCode();
    const user = userEvent.setup();
    renderApp("/escanear");
    await user.type(await screen.findByLabelText("Código del material"), "isk-her-001");
    await user.click(screen.getByRole("button", { name: "Buscar" }));
    expect(await screen.findByRole("heading", { name: "ISK-HER-001" })).toBeInTheDocument();
  });

  it("accepts a pasted label URL in the manual field", async () => {
    useAuthenticatedApi("viewer");
    serveByCode();
    const user = userEvent.setup();
    renderApp("/escanear");
    await user.type(await screen.findByLabelText("Código del material"), "https://inv.example.com/materiales/codigo/ISK-HER-001");
    await user.click(screen.getByRole("button", { name: "Buscar" }));
    expect(await screen.findByRole("heading", { name: "ISK-HER-001" })).toBeInTheDocument();
  });

  it("shows a friendly message when the code does not exist", async () => {
    useAuthenticatedApi("viewer");
    serveByCode();
    const user = userEvent.setup();
    renderApp("/escanear");
    await user.type(await screen.findByLabelText("Código del material"), "ISK-XXX-999");
    await user.click(screen.getByRole("button", { name: "Buscar" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("No se encontró ningún material con el código ISK-XXX-999.");
  });

  it("rejects values that are not codes without calling the API", async () => {
    useAuthenticatedApi("viewer");
    const user = userEvent.setup();
    renderApp("/escanear");
    await user.type(await screen.findByLabelText("Código del material"), "hola mundo");
    await user.click(screen.getByRole("button", { name: "Buscar" }));
    expect(await screen.findByText("Ese valor no parece un código de material.")).toBeInTheDocument();
  });

  it("resolves a code read by the camera (URL form) and stops the camera on leave", async () => {
    useAuthenticatedApi("viewer");
    serveByCode();
    const { unmount } = renderApp("/escanear");
    await waitFor(() => expect(scanner.onCode).not.toBeNull());
    expect(await screen.findByRole("button", { name: /linterna/i })).toBeInTheDocument();
    scanner.onCode?.("https://inv.example.com/materiales/codigo/ISK-HER-001");
    expect(await screen.findByRole("heading", { name: "ISK-HER-001" })).toBeInTheDocument();
    unmount();
    expect(scanner.stop).toHaveBeenCalled();
  });

  it("explains a denied camera permission and keeps manual entry available", async () => {
    useAuthenticatedApi("viewer");
    scanner.start.mockRejectedValue(new CameraError("denied", "denied"));
    renderApp("/escanear");
    expect(await screen.findByText(/No hay permiso para usar la cámara/)).toBeInTheDocument();
    expect(screen.getByLabelText("Código del material")).toBeInTheDocument();
  });

  it("explains when there is no camera", async () => {
    useAuthenticatedApi("viewer");
    scanner.start.mockRejectedValue(new CameraError("no-camera", "none"));
    renderApp("/escanear");
    expect(await screen.findByText(/No se encontró una cámara/)).toBeInTheDocument();
  });
});

describe("printed label URL", () => {
  it("redirects /materiales/codigo/:code to the item detail", async () => {
    useAuthenticatedApi("viewer");
    serveByCode();
    renderApp("/materiales/codigo/isk-her-001");
    expect(await screen.findByRole("heading", { name: "ISK-HER-001" })).toBeInTheDocument();
  });

  it("shows not found for an unknown code", async () => {
    useAuthenticatedApi("viewer");
    serveByCode();
    renderApp("/materiales/codigo/ISK-NOPE-001");
    expect(await screen.findByText("Material no encontrado")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Escanear otro código" })).toBeInTheDocument();
  });
});
