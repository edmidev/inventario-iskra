import type { ItemStock, Location } from "../../lib/api/schemas";
import { locationPath } from "../catalog/locationPath";

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

export interface LocationChoice {
  value: string;
  label: string;
}

/**
 * Locations a consumable movement can target. `onlyStocked` limits the list to places holding stock
 * (salida, traslado origin); otherwise every catalog location is offered, so stock can start somewhere new.
 * Locations with stock show how much they hold.
 */
export function locationChoices(args: {
  locations: readonly Location[];
  stocks: readonly ItemStock[];
  unit: string | null;
  onlyStocked?: boolean;
  exclude?: string;
}): LocationChoice[] {
  const { locations, stocks, unit, onlyStocked = false, exclude } = args;
  const suffix = unit ? ` ${unit}` : "";
  const label = (id: number, fallback: string) => {
    const quantity = stockAt(stocks, id);
    const name = locationPath(locations as Location[], id) ?? fallback;
    return quantity > 0 ? `${name} · ${quantity}${suffix}` : name;
  };
  const rows = stocks.map((row) => ({ id: row.location_id, name: row.location_name }));
  const all = onlyStocked
    ? rows
    : [...locations.map((l) => ({ id: l.id, name: l.name })), ...rows.filter((row) => !locations.some((l) => l.id === row.id))];
  return all.filter((entry) => String(entry.id) !== exclude).map((entry) => ({ value: String(entry.id), label: label(entry.id, entry.name) }));
}

/** Readable name of a location, falling back to the name carried by the stock row. */
export function locationDisplayName(locations: readonly Location[], stocks: readonly ItemStock[], id: number | string): string {
  const numeric = Number(id);
  return locationPath(locations as Location[], numeric) ?? stocks.find((row) => row.location_id === numeric)?.location_name ?? "";
}

/** For tests: forget the remembered locations. */
export const forgetRememberedLocations = () => lastLocationByItem.clear();
