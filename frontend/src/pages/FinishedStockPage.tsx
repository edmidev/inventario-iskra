import { Link, useSearchParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { PagedList } from "../components/molecules/PagedList";
import { SearchInput } from "../components/molecules/SearchInput";
import { useFinishedStock } from "../features/executions/api/executions";
import { describeError } from "../lib/api/errors";

export function FinishedStockPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const statusParam = params.get("status") ?? "available";
  const status = statusParam === "delivered" || statusParam === "all" ? statusParam : "available";
  const query = useFinishedStock(status === "all" ? "" : status, q);
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];

  function update(key: string, value: string) {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value); else next.delete(key);
    setParams(next, { replace: true });
  }

  return <div className="flex flex-col gap-4">
    <Link to="/mas" className="min-h-11 font-semibold text-accent">Más</Link>
    <div><h1 className="text-2xl font-bold">Productos terminados</h1><p className="text-sm text-ink-soft">Unidades fabricadas con código propio, separadas de los insumos y herramientas.</p></div>
    <div className="grid gap-3 sm:grid-cols-[1fr_12rem]">
      <SearchInput label="Buscar producto terminado" placeholder="Código o proyecto" value={q} onChange={(value) => update("q", value)} />
      <label className="flex flex-col gap-1 text-sm font-medium">Estado del producto
        <select value={status} onChange={(event) => update("status", event.target.value)} className="min-h-11 rounded-xl border border-line bg-surface px-3">
          <option value="available">Disponibles</option><option value="delivered">Entregados</option><option value="all">Todos</option>
        </select>
      </label>
    </div>
    <PagedList label="Productos terminados" status={query.status} errorMessage={query.error ? describeError(query.error) : undefined} isEmpty={rows.length === 0} emptyTitle="Sin productos terminados" emptyDescription="Las unidades aparecerán cuando se registre la producción de una ejecución." hasMore={query.hasNextPage} isFetchingMore={query.isFetchingNextPage} onLoadMore={() => void query.fetchNextPage()} onRetry={() => void query.refetch()}>
      {rows.map((row) => <li key={row.id} className="flex flex-col gap-2 rounded-card border border-line bg-surface p-4 text-sm">
        <div className="flex items-center justify-between gap-2"><strong className="font-mono text-base">{row.code}</strong><Badge tone={row.status === "available" ? "ok" : "neutral"}>{row.status === "available" ? "Disponible" : "Entregado"}</Badge></div>
        <p>{row.project_name} · <Link to="/ejecuciones" className="font-semibold text-accent">Ejecución #{row.execution_id}</Link></p>
        <p className="text-ink-soft">Fabricado: {new Date(row.produced_at).toLocaleDateString("es-VE", { timeZone: "America/Caracas" })}</p>
        {row.status === "delivered" && <p>Cliente: <span>{row.customer_name}</span> · Comprobante <span className="font-mono">{row.receipt}</span></p>}
      </li>)}
    </PagedList>
  </div>;
}
