import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Select } from "../components/atoms/Select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/atoms/Tabs";
import { OpsRow } from "../components/molecules/OpsRow";
import { PagedList } from "../components/molecules/PagedList";
import { StatusBadge } from "../components/molecules/ItemBadges";
import { usePermissions } from "../features/auth/session";
import { useDueMaintenance, useOpenFailures } from "../features/maintenance/api/maintenance";
import { CloseFailureSheet } from "../features/maintenance/components/MaintenanceSheets";
import { describeError } from "../lib/api/errors";
import type { MaintenanceRecord } from "../lib/api/schemas";
import { formatDate } from "../lib/format";

const windows = [
  { value: "0", label: "Vencidos" },
  { value: "7", label: "Vencidos y próximos 7 días" },
  { value: "30", label: "Vencidos y próximos 30 días" },
  { value: "90", label: "Vencidos y próximos 90 días" },
];

function OpenFailures() {
  const { canOperate } = usePermissions();
  const query = useOpenFailures();
  const [closing, setClosing] = useState<MaintenanceRecord | null>(null);
  const records = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <>
      <PagedList
        label="Fallas abiertas"
        status={query.status}
        errorMessage={query.error ? describeError(query.error) : undefined}
        isEmpty={records.length === 0}
        emptyTitle="No hay fallas abiertas"
        emptyDescription="Todo el material reportado está operando."
        hasMore={query.hasNextPage}
        isFetchingMore={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
        onRetry={() => void query.refetch()}
      >
        {records.map((record) => (
          <OpsRow
            key={record.id}
            itemId={record.item_id}
            code={record.item_code}
            description={record.item_description}
            aside={<StatusBadge status={record.item_status} />}
            action={
              canOperate && (
                <Button aria-label={`Cerrar falla de ${record.item_code}`} onClick={() => setClosing(record)}>
                  Cerrar falla
                </Button>
              )
            }
          >
            <span className="text-ink">{record.description}</span>
            <span>
              Reportada {formatDate(record.reported_at)}
              {record.destination ? ` · ${record.destination}` : ""}
            </span>
          </OpsRow>
        ))}
      </PagedList>
      {closing && <CloseFailureSheet record={closing} open onOpenChange={(open) => !open && setClosing(null)} />}
    </>
  );
}

function DueList({ withinDays }: { withinDays: number }) {
  const query = useDueMaintenance(withinDays);
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  return (
    <PagedList
      label="Mantenimientos por vencer"
      status={query.status}
      errorMessage={query.error ? describeError(query.error) : undefined}
      isEmpty={rows.length === 0}
      emptyTitle={withinDays === 0 ? "No hay mantenimientos vencidos" : "Nada por vencer en ese plazo"}
      hasMore={query.hasNextPage}
      isFetchingMore={query.isFetchingNextPage}
      onLoadMore={() => void query.fetchNextPage()}
      onRetry={() => void query.refetch()}
    >
      {rows.map((row) => (
        <OpsRow
          key={row.item_id}
          itemId={row.item_id}
          code={row.code}
          description={row.description}
          aside={row.maintenance_overdue ? <Badge tone="bad">Vencido</Badge> : <Badge tone="warn">Próximo</Badge>}
        >
          <span>
            {row.maintenance_overdue ? "Venció" : "Vence"} {formatDate(row.maintenance_due_at)}
          </span>
          <span>
            {row.category_name}
            {row.location_name ? ` · ${row.location_name}` : ""} · cada {row.maintenance_interval_days} días
          </span>
        </OpsRow>
      ))}
    </PagedList>
  );
}

export function MaintenancePage() {
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "due" ? "due" : "open";
  const within = windows.some((w) => w.value === params.get("within")) ? (params.get("within") as string) : "0";
  const set = (next: Record<string, string>) => setParams({ ...Object.fromEntries(params), ...next }, { replace: true });

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-bold">Mantenimiento</h1>
      <Tabs value={tab} onValueChange={(value) => set({ tab: value })}>
        <TabsList label="Secciones de mantenimiento">
          <TabsTrigger value="open">Fallas abiertas</TabsTrigger>
          <TabsTrigger value="due">Por vencer</TabsTrigger>
        </TabsList>
        <TabsContent value="open" className="pt-4">
          <OpenFailures />
        </TabsContent>
        <TabsContent value="due" className="flex flex-col gap-3 pt-4">
          <Select
            aria-label="Plazo"
            value={within}
            onValueChange={(value) => set({ within: value || "0" })}
            placeholder="Plazo"
            options={windows}
          />
          <DueList withinDays={Number(within)} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
