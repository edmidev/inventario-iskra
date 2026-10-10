import { Link, Navigate, useParams } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { EmptyState, ErrorState, LoadingState } from "../components/molecules/StateMessage";
import { useItemByCode } from "../features/scan/api/resolveCode";
import { extractItemCode } from "../features/scan/parseScan";
import { ApiError, describeError } from "../lib/api/errors";

/** Target of printed QR labels: resolves the code and redirects to the item detail. */
export function ResolveCodePage() {
  const { code: raw = "" } = useParams();
  const code = extractItemCode(raw) ?? raw;
  const query = useItemByCode(code);

  if (query.isPending) return <LoadingState label="Buscando material" />;
  if (query.isSuccess) return <Navigate to={`/materiales/${query.data.id}`} replace />;
  if (query.error instanceof ApiError && query.error.status === 404) {
    return (
      <EmptyState
        title="Material no encontrado"
        description={`No existe un material con el código ${code}. Puede haber sido eliminado.`}
        action={
          <Link to="/escanear">
            <Button variant="primary">Escanear otro código</Button>
          </Link>
        }
      />
    );
  }
  return <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />;
}
