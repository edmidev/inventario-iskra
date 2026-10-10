import { Link, Outlet } from "react-router-dom";
import { Button } from "../../components/atoms/Button";
import { EmptyState } from "../../components/molecules/StateMessage";
import { usePermissions, type Permissions } from "./session";

/** Route guard: renders the nested routes only when the role passes `check`, else a clear "no access". */
export function RequireRole({ check }: { check: (permissions: Permissions) => boolean }) {
  const allowed = check(usePermissions());
  if (allowed) return <Outlet />;
  return (
    <EmptyState
      title="Sin acceso"
      description="Tu rol no tiene permiso para ver esta sección."
      action={
        <Link to="/inicio">
          <Button variant="primary">Ir al inicio</Button>
        </Link>
      }
    />
  );
}
