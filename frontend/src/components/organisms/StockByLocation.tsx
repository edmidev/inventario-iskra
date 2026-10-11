import type { ItemStock } from "../../lib/api/schemas";

interface StockByLocationProps {
  stocks: ItemStock[];
  total: number;
  min: number;
  unit: string | null;
  lowStock: boolean;
}

/** A consumable's quantity per location, its total and the low-stock warning. */
export function StockByLocation({ stocks, total, min, unit, lowStock }: StockByLocationProps) {
  const suffix = unit ? ` ${unit}` : "";
  return (
    <section aria-label="Stock por ubicación" className="rounded-card border border-line bg-surface">
      <h2 className="border-b border-line px-4 py-3 font-semibold">Stock por ubicación</h2>
      {stocks.length === 0 ? (
        <p className="px-4 py-4 text-sm text-ink-soft">No hay existencias en ninguna ubicación.</p>
      ) : (
        <ul aria-label="Existencias por ubicación">
          {stocks.map((row) => (
            <li key={row.location_id} className="flex min-h-12 items-center justify-between gap-3 border-b border-line px-4 py-2 last:border-b-0">
              <span className="min-w-0 break-words">{row.location_name}</span>
              <span className="shrink-0 font-semibold tabular-nums">
                {row.quantity}
                {suffix}
              </span>
            </li>
          ))}
        </ul>
      )}
      <footer className="flex flex-col gap-1 rounded-b-card border-t border-line bg-sunken px-4 py-3">
        <p className="flex items-center justify-between gap-3">
          <span className="text-sm font-semibold">Total</span>
          <strong className="text-lg tabular-nums">
            {total}
            {suffix}
          </strong>
        </p>
        <p className="text-xs text-ink-soft">Stock mínimo: {min}{suffix}</p>
        {lowStock && (
          <p className="text-sm font-semibold text-low">
            En o por debajo del mínimo de {min}
            {suffix}.
          </p>
        )}
      </footer>
    </section>
  );
}
