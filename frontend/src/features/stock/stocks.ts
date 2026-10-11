import type { ItemStock } from "../../lib/api/schemas";

/** Quantity held at a location; 0 when the location has no stock row. */
export function stockAt(stocks: readonly ItemStock[], locationId: number | string | null): number {
  if (locationId === null || locationId === "") return 0;
  const id = Number(locationId);
  return stocks.find((row) => row.location_id === id)?.quantity ?? 0;
}

export function totalStock(stocks: readonly ItemStock[]): number {
  return stocks.reduce((sum, row) => sum + row.quantity, 0);
}

/** Where a consumable's stock is, in a few words: the location name, "en N ubicaciones" or "Sin existencias". */
export function stockPlaces(stocks: readonly ItemStock[]): string {
  if (stocks.length === 0) return "Sin existencias";
  if (stocks.length === 1) return stocks[0].location_name;
  return `en ${stocks.length} ubicaciones`;
}

/** The location a quick movement should preselect: the only stocked one, else the last one used. */
export function defaultLocation(stocks: readonly ItemStock[], lastUsed: string | undefined): string {
  if (stocks.length === 1) return String(stocks[0].location_id);
  return lastUsed ?? "";
}

const lastLocationByItem = new Map<string, string>();

export const rememberLocation = (itemId: string, locationId: string) => void lastLocationByItem.set(itemId, locationId);
export const recallLocation = (itemId: string) => lastLocationByItem.get(itemId);
