import { ErrorState, LoadingState } from "../components/molecules/StateMessage";
import { DashboardView } from "../components/organisms/DashboardView";
import { useDashboardSummary } from "../features/dashboard/api/useDashboardSummary";
import { describeError } from "../lib/api/errors";

export function DashboardPage() {
  const query = useDashboardSummary();
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">Inicio</h1>
      {query.isPending && <LoadingState label="Cargando resumen" />}
      {query.isError && <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />}
      {query.isSuccess && <DashboardView summary={query.data} />}
    </div>
  );
}
