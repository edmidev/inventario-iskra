import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { EmptyState, ErrorState, LoadingState } from "../components/molecules/StateMessage";
import { usePermissions } from "../features/auth/session";
import { useReorderSuggestions } from "../features/purchasing/api/reorder";
import { CreatePurchaseOrderSheet } from "../features/purchasing/components/PurchaseOrderSheets";
import { formatMinor } from "../features/purchasing/money";
import { groupToOrderValues } from "../features/purchasing/reorder";
import { describeError } from "../lib/api/errors";
import type { ReorderGroup } from "../lib/api/schemas";

export function ReorderSuggestionsPage() {
  const { canOperate } = usePermissions();
  const query = useReorderSuggestions();
  const [drafting, setDrafting] = useState<ReorderGroup | null>(null);
  const groups = query.data?.groups ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/ordenes-compra" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Órdenes de compra
        </Link>
        <h1 className="text-2xl font-bold">Sugerencias de compra</h1>
        <p className="text-sm text-ink-soft">
          Consumibles en o bajo su mínimo, agrupados por su proveedor habitual. La cantidad apunta al doble del mínimo, descontando lo que ya viene en camino.
        </p>
      </div>

      {query.isPending && <LoadingState label="Cargando sugerencias" />}
      {query.isError && <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />}
      {query.isSuccess && groups.length === 0 && (
        <EmptyState title="Nada que pedir" description="Ningún consumible bajo su mínimo necesita una orden nueva." />
      )}

      {groups.map((group) => {
        const name = group.supplier_name ?? "Sin proveedor habitual";
        return (
          <section key={group.supplier_id ?? "none"} aria-label={name} className="rounded-card border border-line bg-surface">
            <header className="flex items-center justify-between gap-3 border-b border-line px-4 py-3">
              <h2 className="font-semibold">{name}</h2>
              {canOperate && (
                <Button variant="primary" aria-label={`Crear orden para ${name}`} onClick={() => setDrafting(group)}>
                  <Icon name="plus" className="size-4" /> Crear orden
                </Button>
              )}
            </header>
            <ul>
              {group.lines.map((line) => (
                <li key={line.item_id} className="flex min-h-14 items-center justify-between gap-3 border-b border-line px-4 py-2 last:border-b-0">
                  <span className="min-w-0">
                    <span className="block font-mono text-sm font-semibold text-accent">{line.code}</span>
                    <span className="block truncate text-sm">{line.description}</span>
                    <span className="block text-xs text-ink-soft">
                      Stock {line.stock_current} / mín. {line.stock_min}
                      {line.pending_ordered > 0 ? ` · ${line.pending_ordered} en camino` : ""}
                    </span>
                  </span>
                  <span className="shrink-0 text-right text-sm">
                    <span className="block font-semibold tabular-nums">
                      Pedir {line.suggested_qty} {line.unit}
                    </span>
                    {line.last_unit_cost_minor !== null && line.last_currency && (
                      <span className="block text-xs text-ink-soft">Último: {formatMinor(line.last_unit_cost_minor, line.last_currency)}</span>
                    )}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        );
      })}

      {canOperate && drafting && (
        <CreatePurchaseOrderSheet open onOpenChange={(open) => !open && setDrafting(null)} initialValues={groupToOrderValues(drafting)} />
      )}
    </div>
  );
}
