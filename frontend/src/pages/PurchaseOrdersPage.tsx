import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Select } from "../components/atoms/Select";
import { PagedList } from "../components/molecules/PagedList";
import { SearchInput } from "../components/molecules/SearchInput";
import { usePermissions } from "../features/auth/session";
import { usePurchaseOrders } from "../features/purchasing/api/purchaseOrders";
import { CreatePurchaseOrderSheet } from "../features/purchasing/components/PurchaseOrderSheets";
import { orderStatusLabels, orderStatusOptions, orderStatusTones } from "../features/purchasing/labels";
import { formatMinor } from "../features/purchasing/money";
import { useAllSuppliers } from "../features/suppliers/api/suppliers";
import { describeError } from "../lib/api/errors";
import { purchaseOrderStatusSchema } from "../lib/api/schemas";
import { formatDate } from "../lib/format";

export function PurchaseOrdersPage() {
  const { canOperate } = usePermissions();
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const statusParam = params.get("status") ?? "";
  const status = purchaseOrderStatusSchema.safeParse(statusParam).success ? statusParam : "";
  const supplierParam = params.get("supplier_id") ?? "";
  const supplierId = /^\d+$/.test(supplierParam) ? supplierParam : "";
  const set = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  };

  const suppliers = useAllSuppliers();
  const supplierOptions = (suppliers.data?.pages.flatMap((p) => p.data) ?? []).map((s) => ({ value: String(s.id), label: s.name }));
  const query = usePurchaseOrders({ status, supplierId, q });
  const [creating, setCreating] = useState(false);
  const orders = query.data?.pages.flatMap((page) => page.data) ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/mas" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Más
        </Link>
        <div className="flex items-center justify-between gap-3">
          <h1 className="text-2xl font-bold">Órdenes de compra</h1>
          {canOperate && (
            <Button variant="primary" onClick={() => setCreating(true)}>
              <Icon name="plus" className="size-4" /> Nueva
            </Button>
          )}
        </div>
        <Link to="/ordenes-compra/sugerencias" className="inline-flex min-h-11 items-center text-sm font-semibold text-accent">
          Sugerencias de compra
        </Link>
      </div>

      <div className="flex flex-col gap-2">
        <SearchInput value={q} onChange={(value) => set("q", value)} label="Buscar órdenes" placeholder="Proveedor o número de orden" />
        <div className="grid grid-cols-2 gap-2">
          <Select aria-label="Filtrar por estado" value={status} onValueChange={(value) => set("status", value)} options={orderStatusOptions} placeholder="Todos los estados" clearLabel="Todos los estados" />
          <Select aria-label="Filtrar por proveedor" value={supplierId} onValueChange={(value) => set("supplier_id", value)} options={supplierOptions} placeholder="Todos los proveedores" clearLabel="Todos los proveedores" />
        </div>
      </div>

      <PagedList
        label="Órdenes de compra"
        status={query.status}
        errorMessage={query.error ? describeError(query.error) : undefined}
        isEmpty={orders.length === 0}
        emptyTitle="Sin órdenes de compra"
        emptyDescription={q || status || supplierId ? "Prueba con otra búsqueda o quita filtros." : "Crea la primera para pedir material a un proveedor."}
        hasMore={query.hasNextPage}
        isFetchingMore={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
        onRetry={() => void query.refetch()}
      >
        {orders.map((order) => (
          <li key={order.id}>
            <Link
              to={`/ordenes-compra/${order.id}`}
              aria-label={`Orden #${order.id} de ${order.supplier_name}`}
              className="flex min-h-14 flex-col gap-1 rounded-card border border-line bg-surface p-4 hover:bg-sunken"
            >
              <span className="flex items-start justify-between gap-3">
                <span className="min-w-0 font-semibold break-words">
                  #{order.id} · {order.supplier_name}
                </span>
                <Badge tone={orderStatusTones[order.status]}>{orderStatusLabels[order.status]}</Badge>
              </span>
              <span className="text-sm text-ink-soft">
                {order.lines.length} {order.lines.length === 1 ? "línea" : "líneas"} · {formatMinor(order.total_minor, order.currency)}
                {order.expected_at ? ` · entrega ${formatDate(order.expected_at)}` : ""}
              </span>
            </Link>
          </li>
        ))}
      </PagedList>

      {canOperate && <CreatePurchaseOrderSheet open={creating} onOpenChange={setCreating} />}
    </div>
  );
}
