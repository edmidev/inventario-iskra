import type { Currency } from "../../lib/api/schemas";

/** Largest amount (in minor units) we accept so sums stay exact integers. */
const MAX_MINOR = Number.MAX_SAFE_INTEGER;

/**
 * Converts decimal text ("12", "12.5", "12,50") to integer minor units without going through floats.
 * Returns null when the text is not a non-negative amount with at most two decimals.
 */
export function parseMinorUnits(text: string): number | null {
  const match = /^(\d+)(?:[.,](\d{1,2}))?$/.exec(text.trim());
  if (!match) return null;
  const minor = Number(match[1]) * 100 + Number((match[2] ?? "").padEnd(2, "0") || "0");
  return Number.isSafeInteger(minor) && minor <= MAX_MINOR ? minor : null;
}

/** Integer minor units back to editable decimal text: 1250 -> "12.50". */
export function minorToInput(minor: number): string {
  const whole = Math.trunc(minor / 100);
  const cents = String(minor % 100).padStart(2, "0");
  return `${whole}.${cents}`;
}

export const currencyOptions: { value: Currency; label: string }[] = [
  { value: "USD", label: "Dólares (US$)" },
  { value: "VES", label: "Bolívares (Bs.)" },
];

const symbols: Record<Currency, string> = { USD: "US$", VES: "Bs." };
const amountFormat = new Intl.NumberFormat("es-MX", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** "US$ 1,234.50" / "Bs. 1,234.50" from minor units, for display only. */
export function formatMinor(minor: number, currency: Currency): string {
  return `${symbols[currency]} ${amountFormat.format(minor / 100)}`;
}
