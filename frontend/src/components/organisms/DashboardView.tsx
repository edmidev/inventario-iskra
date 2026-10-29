import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { Badge } from "../atoms/Badge";
import { StatCard } from "../molecules/StatCard";
import { stockBreakdownText } from "../../features/stock/stocks";
import { formatDate } from "../../lib/format";
import type { DashboardSummary } from "../../lib/api/schemas";

function AlertList({
  title,
  count,
  empty,
  children,
  seeAllTo,
}: {
  title: string;
  count: number;
  empty: string;
  children: ReactNode;
  seeAllTo?: string;
}) {
  return (
    <section aria-label={title} className="rounded-card border border-line bg-surface">
      <header className="flex items-center justify-between gap-2 border-b border-line px-4 py-3">
        <h2 className="font-semibold">{title}</h2>
        <div className="flex items-center gap-3">
          <Badge tone={count > 0 ? "bad" : "ok"}>{count}</Badge>
          {seeAllTo && count > 0 && (
            <Link to={seeAllTo} className="-my-2 inline-flex min-h-11 items-center px-1 text-sm font-semibold text-accent">
              Ver todo
            </Link>
          )}
        </div>
      </header>
      {count === 0 ? <p className="px-4 py-5 text-sm text-ink-soft">{empty}</p> : <ul>{children}</ul>}
    </section>
  );
}

function Row({ id, code, description, detail, aside }: { id: string; code: string; description: string; detail?: string; aside: ReactNode }) {
  return (
    <li className="border-b border-line last:border-b-0">
      <Link to={`/materiales/${id}`} className="flex min-h-14 items-center justify-between gap-3 px-4 py-2 hover:bg-sunken">
        <span className="min-w-0">
          <span className="block font-mono text-sm font-semibold text-accent">{code}</span>
          <span className="block truncate text-sm">{description}</span>
          {detail && <span className="block truncate text-xs text-ink-soft">{detail}</span>}
        </span>
        <span className="shrink-0 text-right text-sm text-ink-soft">{aside}</span>
      </Link>
    </li>
  );
}

export function DashboardView({ summary }: { summary: DashboardSummary }) {
  const { items, assets, low_stock, loans, maintenance } = summary;
  return (
    <div className="flex flex-col gap-5">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatCard label="Total de materiales" value={items.total} to="/materiales" />
        <StatCard label="Disponibles" value={assets.available} tone="ok" to="/materiales?availability=available" />
        <StatCard label="Prestados" value={loans.active} tone="warn" to="/prestamos" />
        <StatCard label="En reparación o inoperativos" value={assets.in_repair + assets.inoperative} tone="bad" to="/mantenimiento" />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <AlertList title="Stock bajo" count={low_stock.count} empty="Todo el stock está en orden." seeAllTo="/stock-bajo">
          {low_stock.top.map((row) => (
            <Row
              key={row.id}
              id={row.id}
              code={row.code}
              description={row.description}
              detail={row.stocks.length > 1 ? stockBreakdownText(row.stocks) : (row.location_name ?? undefined)}
              aside={
                <>
                  <span className="block font-semibold text-low tabular-nums">
                    {row.stock_current} / {row.stock_min} {row.unit}
                  </span>
                  {row.pending_ordered > 0 && <span className="block text-xs">{row.pending_ordered} en camino</span>}
                </>
              }
            />
          ))}
        </AlertList>

        <AlertList title="Préstamos vencidos" count={loans.overdue} empty="No hay préstamos vencidos." seeAllTo="/prestamos?overdue=true">
          {loans.overdue_top.map((row) => (
            <Row
              key={row.item_id}
              id={row.item_id}
              code={row.code}
              description={row.description}
              aside={
                <>
                  <span className="block font-medium text-ink">{row.assignee_name}</span>
                  <span className="block text-bad">Venció {formatDate(row.due_at)}</span>
                </>
              }
            />
          ))}
        </AlertList>

        <AlertList title="Mantenimiento vencido" count={maintenance.overdue} empty="No hay mantenimientos vencidos." seeAllTo="/mantenimiento?tab=due">
          {maintenance.overdue_top.map((row) => (
            <Row
              key={row.item_id}
              id={row.item_id}
              code={row.code}
              description={row.description}
              aside={<span className="text-bad">Venció {formatDate(row.maintenance_due_at)}</span>}
            />
          ))}
        </AlertList>
      </div>
    </div>
  );
}
