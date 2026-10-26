import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Link } from "react-router-dom";
import { usePermissions, useSession } from "../features/auth/session";
import { roleLabels } from "../features/users/userForm";

const operations = [
  { to: "/stock-bajo", label: "Stock bajo", hint: "Consumibles por reponer" },
  { to: "/prestamos", label: "Préstamos", hint: "Material prestado y vencido" },
  { to: "/mantenimiento", label: "Mantenimiento", hint: "Fallas abiertas y mantenimientos por vencer" },
  { to: "/proveedores", label: "Proveedores", hint: "Contactos y proveedores activos" },
  { to: "/ordenes-compra", label: "Órdenes de compra", hint: "Pedidos, recepciones y costos" },
];

const administration = [
  { to: "/admin/usuarios", label: "Usuarios", hint: "Altas, roles y contraseñas" },
  { to: "/admin/categorias", label: "Categorías", hint: "Nombres y prefijos de código" },
  { to: "/admin/ubicaciones", label: "Ubicaciones", hint: "Bodegas, contenedores y personas" },
];

function LinkList({ label, entries }: { label: string; entries: { to: string; label: string; hint: string }[] }) {
  return (
    <nav aria-label={label} className="flex flex-col overflow-hidden rounded-card border border-line bg-surface">
      {entries.map((entry) => (
        <Link
          key={entry.to}
          to={entry.to}
          className="flex min-h-14 items-center justify-between gap-3 border-b border-line px-4 last:border-b-0 hover:bg-sunken"
        >
          <span>
            <span className="block font-semibold">{entry.label}</span>
            <span className="block text-sm text-ink-soft">{entry.hint}</span>
          </span>
          <Icon name="chevron" className="size-4 text-ink-soft" />
        </Link>
      ))}
    </nav>
  );
}

export function MorePage() {
  const { isAdmin } = usePermissions();
  const { state, signOut } = useSession();
  const user = state.user;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">Más</h1>
      <LinkList label="Operaciones" entries={operations} />
      {isAdmin && (
        <>
          <h2 className="-mb-2 text-sm font-semibold text-ink-soft uppercase">Administración</h2>
          <LinkList label="Administración" entries={administration} />
        </>
      )}
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
