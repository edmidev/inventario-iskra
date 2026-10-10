import { Link } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { useToast } from "../components/molecules/Toast";
import { ChangePasswordForm } from "../components/organisms/UserForms";
import { useChangePassword } from "../features/account/api/account";
import { useSession } from "../features/auth/session";
import { roleLabels } from "../features/users/userForm";

export function AccountPage() {
  const { state, signOut } = useSession();
  const change = useChangePassword();
  const toast = useToast();
  const user = state.user;

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/mas" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Más
        </Link>
        <h1 className="text-2xl font-bold">Mi cuenta</h1>
      </div>
      {user && (
        <section aria-label="Datos de la cuenta" className="flex flex-col gap-2 rounded-card border border-line bg-surface p-4">
          <p className="font-semibold">{user.name}</p>
          <p className="text-sm text-ink-soft">{user.email}</p>
          <div>
            <Badge tone="accent">{roleLabels[user.role]}</Badge>
          </div>
        </section>
      )}
      <section aria-label="Contraseña" className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
        <h2 className="font-semibold">Cambiar contraseña</h2>
        <p className="text-sm text-ink-soft">Al cambiarla se cierran tus sesiones y tendrás que entrar de nuevo.</p>
        <ChangePasswordForm
          onSubmit={async (values) => {
            await change.mutateAsync({ current_password: values.current_password, new_password: values.new_password });
            toast.success("Contraseña cambiada. Inicia sesión de nuevo.");
            await signOut();
          }}
        />
      </section>
      <Button onClick={() => void signOut()}>
        <Icon name="logout" className="size-4" /> Cerrar sesión
      </Button>
    </div>
  );
}
