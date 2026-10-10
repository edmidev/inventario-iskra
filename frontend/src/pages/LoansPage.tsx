import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Select } from "../components/atoms/Select";
import { OpsRow } from "../components/molecules/OpsRow";
import { PagedList } from "../components/molecules/PagedList";
import { SearchInput } from "../components/molecules/SearchInput";
import { usePermissions } from "../features/auth/session";
import { useCatalogLookup } from "../features/catalog/api/catalog";
import { useActiveCustody, type ActiveLoanFilters } from "../features/custody/api/custody";
import { ReturnSheet } from "../features/custody/components/LoanSheets";
import { describeError } from "../lib/api/errors";
import type { Assignment } from "../lib/api/schemas";
import { formatDate } from "../lib/format";

function parse(params: URLSearchParams): ActiveLoanFilters {
  const assignee = Number(params.get("assignee"));
  return {
    overdue: params.get("overdue") === "true",
    assigneeLocationId: Number.isInteger(assignee) && assignee > 0 ? assignee : null,
    q: params.get("q") ?? "",
  };
}

export function LoansPage() {
  const [params, setParams] = useSearchParams();
  const filters = useMemo(() => parse(params), [params]);
  const { canOperate } = usePermissions();
  const catalog = useCatalogLookup();
  const query = useActiveCustody(filters);
  const [returning, setReturning] = useState<Assignment | null>(null);
  const loans = query.data?.pages.flatMap((page) => page.data) ?? [];
  const total = query.data?.pages.at(-1)?.total ?? 0;

  const update = (next: ActiveLoanFilters) => {
    const search = new URLSearchParams();
    if (next.overdue) search.set("overdue", "true");
    if (next.assigneeLocationId) search.set("assignee", String(next.assigneeLocationId));
    if (next.q.trim()) search.set("q", next.q.trim());
    setParams(search, { replace: true });
  };
  const hasFilters = filters.overdue || filters.assigneeLocationId !== null || filters.q.trim() !== "";

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="text-2xl font-bold">Préstamos</h1>
        {query.isSuccess && (
          <p className="text-sm text-ink-soft" aria-live="polite">
            {total === 1 ? "1 préstamo activo" : `${total} préstamos activos`}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <SearchInput
          value={filters.q}
          onChange={(q) => update({ ...filters, q })}
          label="Buscar préstamos"
          placeholder="Código o descripción"
        />
        <div className="grid grid-cols-[1fr_auto] gap-2">
          <Select
            aria-label="Filtrar por responsable"
            value={filters.assigneeLocationId ? String(filters.assigneeLocationId) : ""}
            onValueChange={(value) => update({ ...filters, assigneeLocationId: value ? Number(value) : null })}
            placeholder="Responsable"
            clearLabel="Responsable"
            options={catalog.locations
              .filter((l) => l.kind === "person" || l.kind === "container")
              .map((l) => ({ value: String(l.id), label: l.name }))}
          />
          <Button
            variant={filters.overdue ? "primary" : "secondary"}
            aria-pressed={filters.overdue}
            onClick={() => update({ ...filters, overdue: !filters.overdue })}
          >
            Solo vencidos
          </Button>
        </div>
      </div>

      <PagedList
        label="Préstamos activos"
        status={query.status}
        errorMessage={query.error ? describeError(query.error) : undefined}
        isEmpty={loans.length === 0}
        emptyTitle={hasFilters ? "Sin resultados" : "No hay préstamos activos"}
        emptyDescription={hasFilters ? "Prueba con otro filtro." : "Todo el material está en su lugar."}
        hasMore={query.hasNextPage}
        isFetchingMore={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
        onRetry={() => void query.refetch()}
      >
        {loans.map((loan) => (
          <OpsRow
            key={loan.id}
            itemId={loan.item_id}
            code={loan.item_code}
            description={loan.item_description}
            aside={loan.overdue ? <Badge tone="bad">Vencido</Badge> : <Badge tone="warn">Prestado</Badge>}
            action={
              canOperate && (
                <Button aria-label={`Devolver ${loan.item_code}`} onClick={() => setReturning(loan)}>
                  Devolver
                </Button>
              )
            }
          >
            <span className="font-medium text-ink">{loan.assignee_name}</span>
            <span>
              Desde {formatDate(loan.assigned_at)} ·{" "}
              {loan.due_at ? `Vence ${formatDate(loan.due_at)}` : "Sin fecha de devolución"}
            </span>
          </OpsRow>
        ))}
      </PagedList>

      {returning && (
        <ReturnSheet
          item={{ id: returning.item_id, code: returning.item_code, description: returning.item_description }}
          originName={returning.origin_name}
          open
          onOpenChange={(open) => !open && setReturning(null)}
        />
      )}
    </div>
  );
}
