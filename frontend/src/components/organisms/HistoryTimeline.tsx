import clsx from "clsx";
import type { ReactNode } from "react";
import { Badge, type BadgeTone } from "../atoms/Badge";
import { Button } from "../atoms/Button";
import { EmptyState, ErrorState } from "../molecules/StateMessage";
import { ListSkeleton } from "../molecules/ListSkeleton";
import type { TimelineEvent } from "../../features/history/timeline";
import { formatDate, formatDateTime, movementLabels } from "../../lib/format";
import type { Movement } from "../../lib/api/schemas";

interface HistoryTimelineProps {
  events: TimelineEvent[];
  status: "pending" | "error" | "success";
  errorMessage?: string;
  unit: string | null;
  hasMore: boolean;
  isFetchingMore: boolean;
  locationName: (id: number | null) => string | null;
  onLoadMore: () => void;
  onRetry: () => void;
}

const movementTone: Record<Movement["type"], BadgeTone> = {
  opening: "neutral",
  in: "ok",
  out: "warn",
  adjust: "accent",
  transfer: "accent",
};

function Entry({ at, badge, title, children }: { at: string; badge: ReactNode; title: ReactNode; children?: ReactNode }) {
  return (
    <li className="flex flex-col gap-1 rounded-xl border border-line bg-surface px-4 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex items-center gap-2">
          {badge}
          <span className="font-semibold">{title}</span>
        </span>
        <time dateTime={at} className="text-xs text-ink-soft">
          {formatDateTime(at)}
        </time>
      </div>
      {children}
    </li>
  );
}

function Detail({ children, muted = true }: { children: ReactNode; muted?: boolean }) {
  return <p className={clsx("text-sm", muted && "text-ink-soft")}>{children}</p>;
}

const place = (name: string | null) => name ?? "Sin ubicación";

function MovementEntry({ movement, unit, locationName }: { movement: Movement; unit: string | null; locationName: HistoryTimelineProps["locationName"] }) {
  const suffix = unit ? ` ${unit}` : "";
  const delta = movement.qty > 0 ? `+${movement.qty}` : String(movement.qty);
  // A consumable transfer moves a quantity between two locations; an asset transfer moves the unit (moved_qty is 0).
  const stockTransfer = movement.type === "transfer" && movement.moved_qty > 0;
  const assetTransfer = movement.type === "transfer" && !stockTransfer;
  const where = movement.location_id === null ? null : locationName(movement.location_id);

  let title = "";
  if (stockTransfer) title = `${movement.moved_qty}${suffix}`;
  else if (!assetTransfer) title = `${delta}${suffix}${where ? ` en ${where}` : ""}`;

  return (
    <Entry at={movement.created_at} badge={<Badge tone={movementTone[movement.type]}>{movementLabels[movement.type]}</Badge>} title={title}>
      {movement.type === "transfer" ? (
        <Detail muted={false}>
          {place(locationName(movement.from_location_id))} → {place(locationName(movement.to_location_id))}
        </Detail>
      ) : (
        <Detail>
          Existencias totales: {movement.stock_before} → {movement.stock_after}
          {suffix}
        </Detail>
      )}
      {movement.note && <Detail muted={false}>“{movement.note}”</Detail>}
      <Detail>Por {movement.user_name ?? "Sistema"}</Detail>
    </Entry>
  );
}

function CustodyEntry({ event }: { event: Extract<TimelineEvent, { assignment: unknown }> }) {
  const { assignment } = event;
  if (event.kind === "assigned") {
    return (
      <Entry at={event.at} badge={<Badge tone="warn">Préstamo</Badge>} title={`Prestado a ${assignment.assignee_name}`}>
        <Detail>{assignment.due_at ? `Vence ${formatDate(assignment.due_at)}` : "Sin fecha de devolución"}</Detail>
        {assignment.assign_note && <Detail muted={false}>“{assignment.assign_note}”</Detail>}
        <Detail>Por {assignment.assigned_by_name}</Detail>
      </Entry>
    );
  }
  return (
    <Entry at={event.at} badge={<Badge tone="ok">Devolución</Badge>} title={`Devuelto por ${assignment.assignee_name}`}>
      {assignment.returned_to_name && <Detail>A {assignment.returned_to_name}</Detail>}
      {assignment.return_note && <Detail muted={false}>“{assignment.return_note}”</Detail>}
      <Detail>Por {assignment.returned_by_name ?? "Sistema"}</Detail>
    </Entry>
  );
}

export function HistoryTimeline(props: HistoryTimelineProps) {
  if (props.status === "pending") return <ListSkeleton label="Cargando historial" />;
  if (props.status === "error") {
    return <ErrorState message={props.errorMessage ?? "No se pudo cargar el historial."} onRetry={props.onRetry} />;
  }
  if (props.events.length === 0) {
    return <EmptyState title="Sin movimientos" description="Aún no hay movimientos ni préstamos registrados para este material." />;
  }
  return (
    <div className="flex flex-col gap-3">
      <ol aria-label="Historial del material" className="flex flex-col gap-2">
        {props.events.map((event) =>
          event.kind === "movement" ? (
            <MovementEntry key={event.key} movement={event.movement} unit={props.unit} locationName={props.locationName} />
          ) : (
            <CustodyEntry key={event.key} event={event} />
          ),
        )}
      </ol>
      {props.hasMore && (
        <Button block onClick={props.onLoadMore} disabled={props.isFetchingMore}>
          {props.isFetchingMore ? "Cargando…" : "Cargar más"}
        </Button>
      )}
    </div>
  );
}
