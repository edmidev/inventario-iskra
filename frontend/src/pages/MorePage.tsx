import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon, type IconName } from "../components/atoms/Icon";
import { Link } from "react-router-dom";
import { usePermissions, useSession } from "../features/auth/session";
import { roleLabels } from "../features/users/userForm";

const operations = [
  { to: "/stock-bajo", label: "Stock bajo", hint: "Consumibles por reponer", icon: "box" },
  { to: "/prestamos", label: "Préstamos", hint: "Material prestado y vencido", icon: "share" },
  { to: "/mantenimiento", label: "Mantenimiento", hint: "Fallas y mantenimientos", icon: "flash" },
  { to: "/proveedores", label: "Proveedores", hint: "Contactos y proveedores activos", icon: "truck" },
] as const;

const administration = [
  { permission: "canManageUsers", to: "/admin/usuarios", label: "Usuarios", hint: "Altas, roles y contraseñas", icon: "users" },
  { permission: "canManageCatalogs", to: "/admin/categorias", label: "Categorías", hint: "Nombres y prefijos de código", icon: "tag" },
  { permission: "canManageCatalogs", to: "/admin/ubicaciones", label: "Ubicaciones", hint: "Bodegas, contenedores y personas", icon: "pin" },
  { permission: "canManageRoles", to: "/admin/roles", label: "Roles y permisos", hint: "Accesos por función del taller", icon: "key" },
] as const;

type MenuEntry = { to: string; label: string; hint: string; icon: IconName };

function LinkGrid({ label, entries }: { label: string; entries: readonly MenuEntry[] }) {
  return (
    <section className="flex flex-col gap-2">
      <h2 className="text-sm font-semibold uppercase tracking-wide text-ink-soft">{label}</h2>
      <nav aria-label={label} className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {entries.map((entry) => (
          <Link
            key={entry.to}
            to={entry.to}
            className="flex min-h-36 flex-col items-start gap-3 rounded-card border border-line bg-surface p-4 text-left shadow-sm transition-colors hover:bg-sunken focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
          >
            <span className="flex size-12 shrink-0 items-center justify-center rounded-xl bg-sunken text-accent"><Icon name={entry.icon} className="size-7" /></span>
            <span>
              <span className="block font-semibold">{entry.label}</span>
              <span className="mt-1 block text-xs text-ink-soft">{entry.hint}</span>
            </span>
          </Link>
        ))}
      </nav>
    </section>
  );
}

export function MorePage() {
  const permissions = usePermissions();
  const manufacturingLinks: MenuEntry[] = [
    ...(permissions.canReadProjects ? [{ to: "/proyectos", label: "Proyectos", hint: "Plantillas de fabricación", icon: "copy" as const }] : []),
    ...(permissions.canReadCustomers ? [{ to: "/clientes", label: "Clientes", hint: "Datos y contactos", icon: "users" as const }] : []),
    ...(permissions.canReadExecutions ? [{ to: "/ejecuciones", label: "Ejecuciones", hint: "Trabajos registrados en el taller", icon: "check" as const }] : []),
    ...(permissions.canReadExecutions ? [{ to: "/productos-terminados", label: "Productos terminados", hint: "Unidades disponibles y entregadas", icon: "box" as const }] : []),
  ];
  const administrationLinks = administration.filter((entry) => permissions[entry.permission]);
  const { state, signOut } = useSession();
  const user = state.user;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">Más</h1>
      {permissions.canRead && <LinkGrid label="Operaciones" entries={operations} />}
      {manufacturingLinks.length > 0 && <LinkGrid label="Fabricación" entries={manufacturingLinks} />}
      {administrationLinks.length > 0 && <LinkGrid label="Administración" entries={administrationLinks} />}
      {user && (
        <section aria-label="Cuenta" className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
          <div>
            <p className="font-semibold">{user.name}</p>
            <p className="text-sm text-ink-soft">{user.email}</p>
          </div>
          <div>
            <Badge tone="accent">{roleLabels[user.role]}</Badge>
          </div>
          <Link
            to="/cuenta"
            className="inline-flex min-h-11 items-center justify-center gap-2 rounded-xl border border-line bg-surface px-4 text-sm font-semibold hover:bg-sunken"
          >
            <Icon name="key" className="size-4" /> Mi cuenta y contraseña
          </Link>
          <Button onClick={() => void signOut()}>
            <Icon name="logout" className="size-4" /> Cerrar sesión
          </Button>
        </section>
      )}
    </div>
  );
}
