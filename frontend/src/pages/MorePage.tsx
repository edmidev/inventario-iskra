import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Link } from "react-router-dom";
import { useSession } from "../features/auth/session";

const roleLabels = { admin: "Administrador", operator: "Operador", viewer: "Solo lectura" } as const;

const operations = [
  { to: "/stock-bajo", label: "Stock bajo", hint: "Consumibles por reponer" },
  { to: "/prestamos", label: "Préstamos", hint: "Material prestado y vencido" },
  { to: "/mantenimiento", label: "Mantenimiento", hint: "Fallas abiertas y mantenimientos por vencer" },
];

export function MorePage() {
  const { state, signOut } = useSession();
  const user = state.user;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">Más</h1>
      <nav aria-label="Operaciones" className="flex flex-col overflow-hidden rounded-card border border-line bg-surface">
        {operations.map((entry) => (
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
      {user && (
        <section aria-label="Cuenta" className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
          <div>
            <p className="font-semibold">{user.name}</p>
            <p className="text-sm text-ink-soft">{user.email}</p>
          </div>
          <div>
            <Badge tone="accent">{roleLabels[user.role]}</Badge>
          </div>
          <Button onClick={() => void signOut()}>
            <Icon name="logout" className="size-4" /> Cerrar sesión
          </Button>
        </section>
      )}
    </div>
  );
}
