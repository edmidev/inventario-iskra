import { Badge } from "../atoms/Badge";
import { availabilityLabels, statusLabels } from "../../lib/format";
import type { Item } from "../../lib/api/schemas";

const statusTone = { functional: "ok", inoperative: "bad", in_repair: "warn" } as const;

export function StatusBadge({ status }: { status: Item["status"] }) {
  return <Badge tone={statusTone[status]}>{statusLabels[status]}</Badge>;
}

/** Availability for assets: available / on loan to X (overdue) / unavailable. Consumables have none. */
export function AvailabilityBadge({ item }: { item: Pick<Item, "kind" | "availability" | "on_loan" | "custody"> }) {
  if (item.kind === "consumable") return null;
  if (item.on_loan && item.custody) {
    return (
      <>
        <Badge tone="accent">Prestado a {item.custody.assignee_name}</Badge>
        {item.custody.overdue && <Badge tone="bad">Vencido</Badge>}
      </>
    );
  }
  return <Badge tone={item.availability === "available" ? "ok" : "neutral"}>{availabilityLabels[item.availability]}</Badge>;
}

export function LowStockBadge() {
  return <Badge tone="low">Stock bajo</Badge>;
}
