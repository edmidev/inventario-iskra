import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { dashboardKey } from "../features/dashboard/api/useDashboardSummary";
import { toDateInput } from "../lib/dates";
import type { Item } from "../lib/api/schemas";
import { assignment, item, onLoanItem, summary } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

const serve = (value: Item) => server.use(http.get(api(`/items/${value.id}`), () => HttpResponse.json(value)));

async function pickAssignee(dialog: HTMLElement, user: ReturnType<typeof userEvent.setup>, name = "Juan Gómez") {
  await user.click(within(dialog).getByRole("combobox", { name: /Prestar a/ }));
  await user.click(await screen.findByRole("option", { name }));
}

describe("loan actions", () => {
  it("lends an asset to a person, offering only people and containers", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    let body: Record<string, unknown> = {};
    server.use(
      http.post(api(`/items/${asset.id}/assign`), async ({ request }) => {
        body = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json(assignment({ item_id: asset.id }), { status: 201 });
      }),
    );
    const user = userEvent.setup();
    const { client } = renderApp(`/materiales/${asset.id}`);
    await client.prefetchQuery({ queryKey: dashboardKey, queryFn: () => summary });
    await user.click(await screen.findByRole("button", { name: "Prestar" }));
    const dialog = await screen.findByRole("dialog", { name: "Prestar material" });

    await user.click(within(dialog).getByRole("combobox", { name: /Prestar a/ }));
    expect(screen.queryByRole("option", { name: "Bodega" })).not.toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Bodega › Estante A" })).toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: "Juan Gómez" }));
    expect(within(dialog).getByLabelText("Fecha de devolución")).toHaveAttribute("min", toDateInput());
    await user.type(within(dialog).getByLabelText("Fecha de devolución"), "2099-01-15");
    await user.type(within(dialog).getByLabelText("Nota"), "Obra norte");
    await user.click(within(dialog).getByRole("button", { name: "Registrar préstamo" }));

    expect(await screen.findByText("ISK-HER-001 prestado a Juan Gómez.")).toBeInTheDocument();
    expect(body).toMatchObject({ assignee_location_id: 12, note: "Obra norte" });
    expect(typeof body.due_at).toBe("string");
    expect(new Date(body.due_at as string).getTime()).toBeGreaterThan(Date.now());
    await waitFor(() => expect(client.getQueryState(dashboardKey)?.isInvalidated).toBe(true));
  });

  it("rejects a past due date before calling the API", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Prestar" }));
    const dialog = await screen.findByRole("dialog", { name: "Prestar material" });
    await pickAssignee(dialog, user);
    await user.type(within(dialog).getByLabelText("Fecha de devolución"), "2020-01-01");
    await user.click(within(dialog).getByRole("button", { name: "Registrar préstamo" }));
    expect(await within(dialog).findByText(/no puede ser anterior a hoy/)).toBeInTheDocument();
  });

  it("explains a loan conflict", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    server.use(http.post(api(`/items/${asset.id}/assign`), () => HttpResponse.json(errorBody("already_assigned", "x"), { status: 409 })));
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Prestar" }));
    const dialog = await screen.findByRole("dialog", { name: "Prestar material" });
    await pickAssignee(dialog, user);
    await user.click(within(dialog).getByRole("button", { name: "Registrar préstamo" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("ya está prestado");
  });

  it("maps 422 field errors of the loan", async () => {
    useAuthenticatedApi("operator");
    const asset = item();
    serve(asset);
    server.use(
      http.post(api(`/items/${asset.id}/assign`), () =>
        HttpResponse.json(errorBody("validation_failed", "x", { assignee_location_id: "Ubicación no válida" }), { status: 422 }),
      ),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${asset.id}`);
    await user.click(await screen.findByRole("button", { name: "Prestar" }));
    const dialog = await screen.findByRole("dialog", { name: "Prestar material" });
    await pickAssignee(dialog, user);
    await user.click(within(dialog).getByRole("button", { name: "Registrar préstamo" }));
    expect(await within(dialog).findByText("Ubicación no válida")).toBeInTheDocument();
  });

  it("disables lending for an item that is not functional", async () => {
    useAuthenticatedApi("operator");
    const asset = item({ status: "in_repair", availability: "unavailable" });
    serve(asset);
    renderApp(`/materiales/${asset.id}`);
    const button = await screen.findByRole("button", { name: "Prestar" });
    expect(button).toBeDisabled();
    expect(button).toHaveAccessibleDescription(/funcionales/);
  });

  it("returns a loaned asset with the default location", async () => {
    useAuthenticatedApi("operator");
    const loaned = onLoanItem();
    serve(loaned);
    let body: unknown;
    server.use(
      http.post(api(`/items/${loaned.id}/return`), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(assignment({ id: 3, returned_at: "2026-10-10T10:00:00Z" }));
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${loaned.id}`);
    expect(screen.queryByRole("button", { name: "Prestar" })).not.toBeInTheDocument();
    await user.click(await screen.findByRole("button", { name: "Devolver" }));
    const dialog = await screen.findByRole("dialog", { name: "Registrar devolución" });
    await user.click(within(dialog).getByRole("button", { name: "Registrar devolución" }));
    expect(await screen.findByText("ISK-HER-002 devuelto.")).toBeInTheDocument();
    expect(body).toEqual({});
  });

  it("sends the chosen return location and surfaces not_assigned", async () => {
    useAuthenticatedApi("operator");
    const loaned = onLoanItem();
    serve(loaned);
    let body: unknown;
    server.use(
      http.post(api(`/items/${loaned.id}/return`), async ({ request }) => {
        body = await request.json();
        return HttpResponse.json(errorBody("not_assigned", "x"), { status: 409 });
      }),
    );
    const user = userEvent.setup();
    renderApp(`/materiales/${loaned.id}`);
    await user.click(await screen.findByRole("button", { name: "Devolver" }));
    const dialog = await screen.findByRole("dialog", { name: "Registrar devolución" });
    await user.click(within(dialog).getByRole("combobox", { name: /Devolver a/ }));
    await user.click(await screen.findByRole("option", { name: "Bodega" }));
    await user.click(within(dialog).getByRole("button", { name: "Registrar devolución" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("no está prestado");
    expect(body).toEqual({ return_location_id: 10 });
  });
});
