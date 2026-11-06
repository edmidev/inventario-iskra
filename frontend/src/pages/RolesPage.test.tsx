import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";
import { session } from "../test/fixtures";
import { api, errorBody, renderApp, useAuthenticatedApi } from "../test/render";
import { server } from "../test/server";

function serveRoles(fail = false) {
  let workerGrants: string[] = [];
  server.use(
    http.get(api("/roles"), () => HttpResponse.json({data:[{role:"admin",permissions:["roles:manage","users:manage"]},{role:"worker",permissions:workerGrants}]})),
    http.get(api("/permissions"), () => HttpResponse.json({data:["inventory:read","users:manage","roles:manage"]})),
    http.put(api("/roles/worker/permissions"), async ({request}) => {
      if (fail) return HttpResponse.json(errorBody("internal_error", "Unable to save permissions"),{status:500});
      const body=await request.json() as {permissions:string[]}; workerGrants=body.permissions;
      return HttpResponse.json({role:"worker",permissions:workerGrants});
    }),
  );
  return ()=>workerGrants;
}

describe("role permission configuration",()=>{
  it("saves explicit server grants and protects administrator recovery",async()=>{
    useAuthenticatedApi("admin");const getGrants=serveRoles();const user=userEvent.setup();renderApp("/admin/roles");
    const form=await screen.findByRole("form",{name:"Permisos de Trabajador"});
    await user.click(within(form).getByRole("checkbox",{name:"Consultar inventario"}));
    await user.click(within(form).getByRole("button",{name:"Guardar permisos"}));
    await screen.findByText("Permisos guardados.");expect(getGrants()).toEqual(["inventory:read"]);
    const admin=screen.getByRole("form",{name:"Permisos de Administrador"});
    expect(within(admin).getByRole("checkbox",{name:/Administrar usuarios/})).toBeDisabled();
    expect(within(admin).getByRole("checkbox",{name:/Configurar roles/})).toBeDisabled();
  });
  it("shows a recoverable save failure",async()=>{
    useAuthenticatedApi("admin");serveRoles(true);const user=userEvent.setup();renderApp("/admin/roles");
    const form=await screen.findByRole("form",{name:"Permisos de Trabajador"});await user.click(within(form).getByRole("button",{name:"Guardar permisos"}));
    expect(await within(form).findByRole("alert")).toBeInTheDocument();expect(within(form).getByRole("button",{name:"Guardar permisos"})).toBeEnabled();
  });
  it("blocks an admin role without a server management grant",async()=>{
    useAuthenticatedApi("admin");server.use(http.post(api("/auth/refresh"),()=>HttpResponse.json({...session("admin"),user:{...session("admin").user,permissions:[]}})));
    renderApp("/admin/roles");expect(await screen.findByText("Sin acceso")).toBeInTheDocument();
  });
  it("allows a worker with the explicit role management grant",async()=>{
    useAuthenticatedApi("worker");serveRoles();server.use(http.post(api("/auth/refresh"),()=>HttpResponse.json({...session("worker"),user:{...session("worker").user,permissions:["roles:manage"]}})));
    renderApp("/admin/roles");expect(await screen.findByRole("heading",{name:"Roles y permisos"})).toBeInTheDocument();
  });
});
