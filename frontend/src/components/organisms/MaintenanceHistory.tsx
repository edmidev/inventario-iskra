import clsx from "clsx";
import { Badge } from "../atoms/Badge";
import { Button } from "../atoms/Button";
import { EmptyState, ErrorState } from "../molecules/StateMessage";
import { ListSkeleton } from "../molecules/ListSkeleton";
import { formatDate, formatMoney, maintenanceKindLabels } from "../../lib/format";
import type { Item, MaintenanceRecord } from "../../lib/api/schemas";

interface MaintenanceHistoryProps {
  item: Pick<Item, "maintenance_due_at" | "maintenance_overdue" | "maintenance_interval_days">;
  records: MaintenanceRecord[];
  status: "pending" | "error" | "success";
  errorMessage?: string;
  hasMore: boolean;
  isFetchingMore: boolean;
  onLoadMore: () => void;
  onRetry: () => void;
}

function DueBanner({ item }: Pick<MaintenanceHistoryProps, "item">) {
  if (!item.maintenance_due_at) {
    return (
      <p className="rounded-xl bg-sunken px-4 py-3 text-sm text-ink-soft">
        {item.maintenance_interval_days
          ? "Aún no hay un próximo mantenimiento calculado."
          : "Este material no tiene mantenimiento programado."}
      </p>
    );
  }
  return (
    <p
      role={item.maintenance_overdue ? "alert" : undefined}
      className={clsx(
        "rounded-xl px-4 py-3 text-sm font-medium",
        item.maintenance_overdue ? "bg-bad-soft text-bad" : "bg-accent-soft text-accent",
      )}
    >
      {item.maintenance_overdue
        ? `Mantenimiento vencido desde el ${formatDate(item.maintenance_due_at)}.`
        : `Próximo mantenimiento: ${formatDate(item.maintenance_due_at)}.`}
    </p>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs text-ink-soft">{label}</dt>
      <dd className="text-sm font-medium break-words">{value}</dd>
    </div>
  );
}

function RecordCard({ record }: { record: MaintenanceRecord }) {
  const open = record.status === "open";
  const failure = record.kind === "corrective";
  return (
    <li
      aria-label={open ? "Falla abierta" : undefined}
      className={clsx("flex flex-col gap-2 rounded-xl border px-4 py-3", open ? "border-bad/50 bg-bad-soft" : "border-line bg-surface")}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Badge tone={failure ? "bad" : "accent"}>{maintenanceKindLabels[record.kind]}</Badge>
        <Badge tone={open ? "bad" : "ok"}>{open ? "Abierta" : "Cerrada"}</Badge>
      </div>
      <p className="font-medium">{record.description}</p>
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2">
        <Fact label={failure ? "Reportada" : "Realizado"} value={formatDate(record.reported_at)} />
        {!open && failure && <Fact label="Cerrada" value={formatDate(record.closed_at)} />}
        {record.destination && <Fact label="Destino" value={record.destination} />}
        {record.cost !== null && <Fact label="Costo" value={formatMoney(record.cost)} />}
        {record.next_due_at && <Fact label="Próximo mantenimiento" value={formatDate(record.next_due_at)} />}
        <Fact label="Registró" value={record.reported_by_name} />
        {record.closed_by_name && failure && <Fact label="Cerró" value={record.closed_by_name} />}
      </dl>
      {record.resolution_note && <p className="text-sm">Resolución: {record.resolution_note}</p>}
    </li>
  );
}

export function MaintenanceHistory(props: MaintenanceHistoryProps) {
  let body;
  if (props.status === "pending") {
    body = <ListSkeleton label="Cargando mantenimiento" rows={3} />;
  } else if (props.status === "error") {
    body = <ErrorState message={props.errorMessage ?? "No se pudo cargar el mantenimiento."} onRetry={props.onRetry} />;
  } else if (props.records.length === 0) {
    body = <EmptyState title="Sin registros" description="Aún no hay fallas ni mantenimientos registrados para este material." />;
  } else {
    body = (
      <>
        <ul aria-label="Registros de mantenimiento" className="flex flex-col gap-2">
          {props.records.map((record) => (
            <RecordCard key={record.id} record={record} />
          ))}
        </ul>
        {props.hasMore && (
          <Button block onClick={props.onLoadMore} disabled={props.isFetchingMore}>
            {props.isFetchingMore ? "Cargando…" : "Cargar más"}
          </Button>
        )}
      </>
    );
  }
  return (
    <div className="flex flex-col gap-3">
      <DueBanner item={props.item} />
      {body}
    </div>
  );
}
