import type { ItemStock } from "../../lib/api/schemas";

/** Compact "Taller 5  Maleta 3" breakdown of a consumable's stock per location. */
export function StockChips({ stocks, unit, className }: { stocks: ItemStock[]; unit?: string | null; className?: string }) {
  if (stocks.length === 0) return null;
  return (
    <ul aria-label="Existencias por ubicación" className={`flex flex-wrap gap-1.5 ${className ?? ""}`}>
      {stocks.map((row) => (
        <li key={row.location_id} className="rounded-md bg-sunken px-2 py-0.5">
          {row.location_name} <strong className="font-semibold tabular-nums text-ink">{row.quantity}</strong>
          {unit ? ` ${unit}` : ""}
        </li>
      ))}
    </ul>
  );
}
