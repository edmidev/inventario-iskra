import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { useSession } from "../features/auth/session";

const roleLabels = { admin: "Administrador", operator: "Operador", viewer: "Solo lectura" } as const;

export function MorePage() {
  const { state, signOut } = useSession();
  const user = state.user;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">Más</h1>
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
