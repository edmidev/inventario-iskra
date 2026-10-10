import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Select } from "../components/atoms/Select";
import { ListSkeleton } from "../components/molecules/ListSkeleton";
import { SearchInput } from "../components/molecules/SearchInput";
import { EmptyState, ErrorState } from "../components/molecules/StateMessage";
import { useSession } from "../features/auth/session";
import { useUsers } from "../features/users/api/users";
import { CreateUserSheet, EditUserSheet, ResetPasswordSheet } from "../features/users/components/UserSheets";
import { roleLabels, roleOptions } from "../features/users/userForm";
import { describeError } from "../lib/api/errors";
import type { User } from "../lib/api/schemas";

const activeOptions = [
  { value: "active", label: "Activos" },
  { value: "inactive", label: "Inactivos" },
];

export function UsersPage() {
  const { state } = useSession();
  const query = useUsers();
  const [q, setQ] = useState("");
  const [role, setRole] = useState("");
  const [active, setActive] = useState("");
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [resetting, setResetting] = useState<User | null>(null);

  const users = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (query.data ?? []).filter(
      (u) =>
        (needle === "" || u.name.toLowerCase().includes(needle) || u.email.toLowerCase().includes(needle)) &&
        (role === "" || u.role === role) &&
        (active === "" || (active === "active") === u.active),
    );
  }, [query.data, q, role, active]);

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/mas" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Más
        </Link>
        <div className="flex items-center justify-between gap-3">
          <h1 className="text-2xl font-bold">Usuarios</h1>
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Icon name="plus" className="size-4" /> Nuevo
          </Button>
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <SearchInput value={q} onChange={setQ} label="Buscar usuarios" placeholder="Buscar por nombre o correo" />
        <div className="grid grid-cols-2 gap-2">
          <Select aria-label="Filtrar por rol" value={role} onValueChange={setRole} options={roleOptions} placeholder="Todos los roles" clearLabel="Todos los roles" />
          <Select aria-label="Filtrar por estado" value={active} onValueChange={setActive} options={activeOptions} placeholder="Todos" clearLabel="Todos" />
        </div>
      </div>

      {query.isPending && <ListSkeleton label="Cargando usuarios" />}
      {query.isError && <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />}
      {query.isSuccess && users.length === 0 && (
        <EmptyState title="Sin resultados" description="Prueba con otra búsqueda o quita filtros." />
      )}
      {users.length > 0 && (
        <ul aria-label="Usuarios" className="flex flex-col gap-2">
          {users.map((user) => (
            <li key={user.id} className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-semibold break-words">
                    {user.name}
                    {user.id === state.user?.id && <span className="font-normal text-ink-soft"> (tú)</span>}
                  </p>
                  <p className="text-sm break-all text-ink-soft">{user.email}</p>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <Badge tone={user.role === "admin" ? "accent" : "neutral"}>{roleLabels[user.role]}</Badge>
                  {!user.active && <Badge tone="bad">Inactivo</Badge>}
                </div>
              </div>
              <div className="flex gap-2">
                <Button className="flex-1" aria-label={`Editar ${user.name}`} onClick={() => setEditing(user)}>
                  <Icon name="edit" className="size-4" /> Editar
                </Button>
                <Button className="flex-1" aria-label={`Restablecer contraseña de ${user.name}`} onClick={() => setResetting(user)}>
                  <Icon name="key" className="size-4" /> Contraseña
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <CreateUserSheet open={creating} onOpenChange={setCreating} />
      {editing && <EditUserSheet user={editing} open onOpenChange={(open) => !open && setEditing(null)} />}
      {resetting && <ResetPasswordSheet user={resetting} open onOpenChange={(open) => !open && setResetting(null)} />}
    </div>
  );
}
