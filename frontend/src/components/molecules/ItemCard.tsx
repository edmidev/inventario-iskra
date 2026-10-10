import { Link } from "react-router-dom";
import { Icon } from "../atoms/Icon";
import { AvailabilityBadge, LowStockBadge, StatusBadge } from "./ItemBadges";
import type { Item } from "../../lib/api/schemas";

interface ItemCardProps {
  item: Item;
  categoryName: string | null;
  locationName: string | null;
}

/** Card on phones, table-like row on md+ (see ItemListHeader for the column grid). */
export function ItemCard({ item, categoryName, locationName }: ItemCardProps) {
  return (
    <li>
      <Link
        to={`/materiales/${item.id}`}
        className="grid gap-1.5 rounded-card border border-line bg-surface p-4 transition-colors hover:bg-sunken md:grid-cols-[8rem_1fr_9rem_9rem_12rem_1.5rem] md:items-center md:gap-4 md:rounded-none md:border-x-0 md:border-t-0 md:px-4 md:py-3"
      >
        <span className="font-mono text-sm font-semibold text-accent">{item.code}</span>
        <span className="font-medium">{item.description}</span>
        <span className="text-sm text-ink-soft">{categoryName ?? "—"}</span>
        <span className="text-sm text-ink-soft md:block">
          {item.kind === "consumable" ? (
            <span className="font-semibold text-ink tabular-nums">
              {item.stock_current} {item.unit}
              <span className="font-normal text-ink-soft"> (mín. {item.stock_min})</span>
            </span>
          ) : (
            (locationName ?? "Sin ubicación")
          )}
        </span>
        <span className="flex flex-wrap gap-1.5">
          <StatusBadge status={item.status} />
          <AvailabilityBadge item={item} />
          {item.kind === "consumable" && item.low_stock && <LowStockBadge />}
        </span>
        <Icon name="chevron" className="hidden size-4 text-ink-soft md:block" />
      </Link>
    </li>
  );
}

export function ItemListHeader() {
  return (
    <div
      aria-hidden="true"
      className="hidden grid-cols-[8rem_1fr_9rem_9rem_12rem_1.5rem] gap-4 border-b border-line px-4 py-2 text-xs font-semibold tracking-wide text-ink-soft uppercase md:grid"
    >
      <span>Código</span>
      <span>Descripción</span>
      <span>Categoría</span>
      <span>Ubicación / stock</span>
      <span>Estado</span>
      <span />
    </div>
  );
}
