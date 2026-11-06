/** Human-unit quantities stay numeric on the wire; arithmetic uses integer thousandths. */
export function parseQuantity(text: string, max = 1_000_000_000): number | null {
  const normalized = text.trim().replace(",", ".");
  if (!/^\d+(?:\.\d{1,3})?$/.test(normalized)) return null;
  const [whole, fraction = ""] = normalized.split(".");
  const scaled = BigInt(whole) * 1000n + BigInt(fraction.padEnd(3, "0"));
  if (scaled > BigInt(Math.trunc(max)) * 1000n) return null;
  return Number(scaled) / 1000;
}

export function quantityAfter(current: number, delta: number): number {
  return (Math.round(current * 1000) + Math.round(delta * 1000)) / 1000;
}

/** Per-line half-up rounding, matching the API's integer minor-unit totals. */
export function lineCostMinor(text: string, cost: number): number | null {
  const quantity = parseQuantity(text);
  if (quantity === null || !Number.isInteger(cost) || cost < 0) return null;
  const subtotal = (BigInt(Math.round(quantity * 1000)) * BigInt(cost) + 500n) / 1000n;
  return Number(subtotal);
}
