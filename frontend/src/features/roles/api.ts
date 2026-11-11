import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSession } from "../auth/session";
import { emitSession } from "../../lib/api/session-store";
import { z } from "zod";
import { request } from "../../lib/api/client";
import { listOf, roleSchema, userSchema, type Role } from "../../lib/api/schemas";

const configurationSchema = z.object({ role: roleSchema, permissions: z.array(z.string()) });
export type RoleConfiguration = z.infer<typeof configurationSchema>;
export const rolesKey = ["roles"] as const;
export function useRoles() {
  return useQuery({ queryKey: rolesKey, queryFn: ({ signal }) => request("/roles", listOf(configurationSchema), { signal }).then((r) => r.data) });
}
export function usePermissionCatalog() {
  return useQuery({ queryKey: ["permission-catalog"], queryFn: ({ signal }) => request("/permissions", listOf(z.string()), { signal }).then((r) => r.data) });
}
export function useSavePermissions(role: Role) {
  const client = useQueryClient();
  const { state } = useSession();
  return useMutation({
    mutationFn: (permissions: string[]) => request(`/roles/${role}/permissions`, configurationSchema, { method: "PUT", body: { permissions } }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: rolesKey });
      if (state.user?.role === role) {
        try { emitSession({ type: "session", user: await request("/auth/me", userSchema) }); }
        catch { emitSession({ type: "expired" }); }
      }
    },
  });
}
export const permissionLabels: Record<string, string> = {
  "customers:read": "Consultar clientes",
  "customers:write": "Administrar clientes",
  "projects:read": "Consultar proyectos",
  "projects:write": "Administrar proyectos",
  "executions:read": "Consultar ejecuciones",
  "executions:write": "Registrar y actualizar ejecuciones",
  "inventory:read": "Consultar inventario",
  "operations:write": "Registrar movimientos, préstamos y mantenimiento",
  "items:write": "Crear y modificar artículos",
  "catalogs:manage": "Administrar categorías, ubicaciones y códigos",
  "users:manage": "Administrar usuarios",
  "roles:manage": "Configurar roles y permisos",
};
