import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { EmptyState, ErrorState, LoadingState } from "../components/molecules/StateMessage";
import { FormAlert } from "../components/molecules/FormFooter";
import { permissionLabels, usePermissionCatalog, useRoles, useSavePermissions, type RoleConfiguration } from "../features/roles/api";
import { roleLabels } from "../features/users/userForm";
import { describeError } from "../lib/api/errors";

function RolePermissionsForm({ configuration, catalog }: { configuration: RoleConfiguration; catalog: string[] }) {
  const [selected, setSelected] = useState(configuration.permissions);
  const [saved, setSaved] = useState(false);
  const mutation = useSavePermissions(configuration.role);
  return (
    <form aria-label={`Permisos de ${roleLabels[configuration.role]}`} className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4" onSubmit={(event) => {
      event.preventDefault(); setSaved(false); mutation.mutate(selected, { onSuccess: () => setSaved(true) });
    }}>
      <h2 className="text-lg font-semibold">{roleLabels[configuration.role]}</h2>
      {configuration.role === "admin" && <p className="text-sm text-ink-soft">La administración de usuarios y permisos se conserva para evitar perder el acceso.</p>}
      <fieldset disabled={mutation.isPending} className="flex flex-col gap-1">
        <legend className="sr-only">Accesos permitidos</legend>
        {catalog.map((permission) => {
          const locked = configuration.role === "admin" && ["roles:manage", "users:manage"].includes(permission);
          return <label key={permission} className="flex min-h-11 items-center gap-3 rounded-xl px-2 hover:bg-sunken">
            <input type="checkbox" className="size-5 accent-(--accent)" checked={selected.includes(permission)} disabled={locked} onChange={(event) => {
              setSaved(false); setSelected(event.target.checked ? [...selected, permission] : selected.filter((p) => p !== permission));
            }} />
            <span className="text-sm">{permissionLabels[permission] ?? permission}{locked && <span className="ml-1 text-ink-soft">(protegido)</span>}</span>
          </label>;
        })}
      </fieldset>
      <FormAlert message={mutation.isError ? describeError(mutation.error) : null} />
      {saved && <output className="text-sm text-ink-soft">Permisos guardados.</output>}
      <Button type="submit" variant="primary" disabled={mutation.isPending}>{mutation.isPending ? "Guardando…" : "Guardar permisos"}</Button>
    </form>
  );
}

export function RolesPage() {
  const roles = useRoles(); const catalog = usePermissionCatalog();
  return <div className="flex flex-col gap-4">
    <Link to="/mas" className="min-h-11 text-sm font-semibold text-accent">Más</Link>
    <div><h1 className="text-2xl font-bold">Roles y permisos</h1><p className="text-sm text-ink-soft">Define los accesos de cada función. Los cambios se aplican en la próxima solicitud al sistema.</p></div>
    {(roles.isPending || catalog.isPending) && <LoadingState label="Cargando permisos" />}
    {(roles.isError || catalog.isError) && <ErrorState message={describeError(roles.error ?? catalog.error)} onRetry={() => { void roles.refetch(); void catalog.refetch(); }} />}
    {roles.isSuccess && catalog.isSuccess && (roles.data.length === 0 ? <EmptyState title="Sin roles" /> : roles.data.map((configuration) => <RolePermissionsForm key={configuration.role} configuration={configuration} catalog={catalog.data} />))}
  </div>;
}
