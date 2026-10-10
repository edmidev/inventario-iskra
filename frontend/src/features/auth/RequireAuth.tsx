import { useEffect } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { Spinner } from "../../components/atoms/Spinner";
import { ErrorState } from "../../components/molecules/StateMessage";
import { useSession } from "./session";

export function RequireAuth() {
  const { state, retry } = useSession();
  const location = useLocation();

  const unreachable = state.status === "unreachable";
  // Coming back online is the cue to verify the session again.
  useEffect(() => {
    if (!unreachable) return;
    window.addEventListener("online", retry);
    return () => window.removeEventListener("online", retry);
  }, [unreachable, retry]);

  if (unreachable) {
    return (
      <div className="mx-auto grid min-h-dvh max-w-sm place-items-center px-5">
        <ErrorState message="No se pudo conectar para verificar tu sesión. Revisa tu conexión." onRetry={retry} />
      </div>
    );
  }
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
