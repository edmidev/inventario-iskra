import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Spinner } from "../../components/atoms/Spinner";
import { useSession } from "./session";

export function RequireAuth() {
  const { state } = useSession();
  const location = useLocation();

  if (state.status === "loading") {
    return (
      <div className="grid min-h-dvh place-items-center">
        <Spinner label="Cargando sesión" />
      </div>
    );
  }
  if (state.status === "anonymous") {
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  }
  return <Outlet />;
}
