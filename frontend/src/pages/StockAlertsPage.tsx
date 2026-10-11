import { useState } from "react";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { StockChips } from "../components/molecules/StockChips";
import { OpsRow } from "../components/molecules/OpsRow";
import { PagedList } from "../components/molecules/PagedList";
import { usePermissions } from "../features/auth/session";
import { useStockAlerts } from "../features/stock/api/stock";
import { MovementSheet, type MovementSubject } from "../features/stock/components/StockSheets";
import { describeError } from "../lib/api/errors";
import type { StockAlert } from "../lib/api/schemas";

function Severity({ alert }: { alert: StockAlert }) {
  if (alert.stock_current === 0) return <Badge tone="bad">Agotado</Badge>;
  if (alert.shortage < 0) return <Badge tone="low">Bajo el mínimo</Badge>;
  return <Badge tone="warn">En el mínimo</Badge>;
}

export function StockAlertsPage() {
  const { canOperate } = usePermissions();
  const query = useStockAlerts();
  const [restocking, setRestocking] = useState<MovementSubject | null>(null);
  const alerts = query.data?.pages.flatMap((page) => page.data) ?? [];
  const total = query.data?.pages.at(-1)?.total ?? 0;

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-bold">Stock bajo</h1>
        {query.isSuccess && (
          <p className="text-sm text-ink-soft" aria-live="polite">
            {total === 1 ? "1 consumible necesita reposición" : `${total} consumibles necesitan reposición`}
          </p>
        )}
      </div>
      <PagedList
        label="Consumibles con stock bajo"
        status={query.status}
        errorMessage={query.error ? describeError(query.error) : undefined}
        isEmpty={alerts.length === 0}
        emptyTitle="Todo el stock está en orden"
        emptyDescription="Ningún consumible está en o por debajo de su mínimo."
        hasMore={query.hasNextPage}
        isFetchingMore={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
        onRetry={() => void query.refetch()}
      >
        {alerts.map((alert) => (
          <OpsRow
            key={alert.item_id}
            itemId={alert.item_id}
            code={alert.code}
            description={alert.description}
            aside={
              <>
                <Severity alert={alert} />
                <span className="text-sm font-semibold tabular-nums">
                  {alert.stock_current} / {alert.stock_min} {alert.unit}
                </span>
              </>
            }
            action={
              canOperate && (
                <Button
                  variant="primary"
                  aria-label={`Entrada de ${alert.code}`}
                  onClick={() =>
                    setRestocking({
                      id: alert.item_id,
                      code: alert.code,
                      description: alert.description,
                      unit: alert.unit,
                      stock_current: alert.stock_current,
                      stock_min: alert.stock_min,
                      stocks: alert.stocks,
                    })
                  }
                >
                  <Icon name="plus" className="size-4" /> Entrada
                </Button>
              )
            }
          >
            <StockChips stocks={alert.stocks} unit={alert.unit} />
          </OpsRow>
        ))}
      </PagedList>
      {restocking && (
        <MovementSheet mode="in" subject={restocking} open onOpenChange={(open) => !open && setRestocking(null)} />
      )}
    </div>
  );
}
