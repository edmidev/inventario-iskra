import type { ItemStock } from "../../lib/api/schemas";

/** Compact "Taller 5 · Maleta 3" breakdown of a consumable's stock per location. */
export function StockChips({ stocks, unit, className }: { stocks: ItemStock[]; unit?: string | null; className?: string }) {
  if (stocks.length === 0) return null;
  return (
    <ul aria-label="Existencias por ubicación" className={`flex flex-wrap gap-x-2 gap-y-0.5 ${className ?? ""}`}>
      {stocks.map((row, index) => (
        <li key={row.location_id} className="flex items-center gap-2">
          {index > 0 && (
            <span aria-hidden="true" className="text-ink-soft">
              ·
            </span>
          )}
          <span>
            {row.location_name} <strong className="font-semibold tabular-nums text-ink">{row.quantity}</strong>
            {unit ? ` ${unit}` : ""}
          </span>
        </li>
      ))}
    </ul>
  );
}
