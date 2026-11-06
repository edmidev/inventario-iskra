import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { AppProviders, createQueryClient } from "../providers";
import { session, consumable } from "../test/fixtures";
import { api } from "../test/api";
import { server } from "../test/server";
import { ProjectsPage } from "./ProjectsPage";
import { ProjectForm } from "../features/projects/components/ProjectForm";
import type { Project, ProjectPayload } from "../features/projects/api/projects";
const material = consumable();
const project: Project = {
  id: 1,
  name: "Marmita",
  description: "Acero inoxidable",
  characteristics: ["50 litros"],
  consumables: [{
    item_id: material.id,
    quantity: 1.125
  }],
  tools: [],
  estimated_days: 5,
  created_at: "2026-11-05T00:00:00Z",
  updated_at: "2026-11-05T00:00:00Z"
};
function wrap(content: React.ReactNode) {
  const client = createQueryClient();
  return render(<AppProviders client={client}><MemoryRouter>{content}</MemoryRouter></AppProviders>);
}
describe("project templates", () => {
  it("lists per-unit needs and hides writes without explicit project grant", async () => {
    server.use(http.post(api("/auth/refresh"), () => HttpResponse.json({
      ...session("worker"),
      user: {
        ...session("worker").user,
        permissions: ["projects:read"]
      }
    })), http.get(api("/projects"), () => HttpResponse.json({
      data: [project],
      total: 1,
      limit: 25,
      offset: 0
    })));
    wrap(<ProjectsPage />);
    expect(await screen.findByRole("link", {
      name: "Marmita"
    })).toHaveAttribute("href", "/proyectos/1");
    expect(screen.getByText("5 días · 1 consumibles · 0 herramientas")).toBeInTheDocument();
    expect(screen.queryByRole("button", {
      name: "Nuevo proyecto"
    })).not.toBeInTheDocument();
  });
  it("submits fractional per-unit material requirements without scaling tools or days", async () => {
    server.use(http.post(api("/auth/refresh"), () => HttpResponse.json(session())), http.get(api("/items"), () => HttpResponse.json({
      data: [material],
      total: 1,
      limit: 100,
      offset: 0
    })));
    const save = vi.fn<(payload: ProjectPayload) => Promise<void>>().mockResolvedValue(undefined);
    wrap(<ProjectForm project={project} onSubmit={save} onCancel={() => {}} />);
    const user = userEvent.setup();
    const amount = screen.getByLabelText(/Cantidad 1/);
    await user.clear(amount);
    await user.type(amount, "2.125");
    await user.click(screen.getByRole("button", {
      name: "Guardar cambios"
    }));
    await vi.waitFor(() => expect(save).toHaveBeenCalledWith({
      name: project.name,
      description: project.description,
      characteristics: project.characteristics,
      estimated_days: 5,
      consumables: [{
        item_id: material.id,
        quantity: 2.125
      }],
      tools: []
    }));
  });
  it("rejects a fourth decimal before calling the API", async () => {
    server.use(http.post(api("/auth/refresh"), () => HttpResponse.json(session())), http.get(api("/items"), () => HttpResponse.json({
      data: [material],
      total: 1,
      limit: 100,
      offset: 0
    })));
    const save = vi.fn<(payload: ProjectPayload) => Promise<void>>().mockResolvedValue(undefined);
    wrap(<ProjectForm project={project} onSubmit={save} onCancel={() => {}} />);
    const user = userEvent.setup();
    await user.clear(screen.getByLabelText(/Cantidad 1/));
    await user.type(screen.getByLabelText(/Cantidad 1/), "0.0001");
    await user.click(screen.getByRole("button", {
      name: "Guardar cambios"
    }));
    expect(await screen.findByText("Usa hasta tres decimales.")).toBeInTheDocument();
    expect(save).not.toHaveBeenCalled();
  });
});
