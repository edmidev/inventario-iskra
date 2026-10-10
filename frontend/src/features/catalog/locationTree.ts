import type { Location } from "../../lib/api/schemas";

export interface LocationRow {
  location: Location;
  depth: number;
}

const byName = (a: Location, b: Location) => a.name.localeCompare(b.name, "es");

/** Flattens the parent_id forest into display order (depth-first, siblings by name). */
export function buildLocationRows(locations: Location[]): LocationRow[] {
  const ids = new Set(locations.map((l) => l.id));
  const children = new Map<number | null, Location[]>();
  for (const location of locations) {
    // A parent that is not in the list (should not happen) is treated as a root, never hidden.
    const key = location.parent_id !== null && ids.has(location.parent_id) ? location.parent_id : null;
    children.set(key, [...(children.get(key) ?? []), location]);
  }
  const rows: LocationRow[] = [];
  const seen = new Set<number>();
  const walk = (parent: number | null, depth: number) => {
    for (const location of (children.get(parent) ?? []).sort(byName)) {
      if (seen.has(location.id)) continue;
      seen.add(location.id);
      rows.push({ location, depth });
      walk(location.id, depth + 1);
    }
  };
  walk(null, 0);
  return rows;
}

/** Ids of a location and everything below it: invalid choices for its own parent. */
export function descendantIds(locations: Location[], rootId: number): Set<number> {
  const result = new Set<number>([rootId]);
  let grew = true;
  while (grew) {
    grew = false;
    for (const location of locations) {
      if (location.parent_id !== null && result.has(location.parent_id) && !result.has(location.id)) {
        result.add(location.id);
        grew = true;
      }
    }
  }
  return result;
}

export const locationKindLabels: Record<Location["kind"], string> = {
  storage: "Almacén",
  container: "Contenedor",
  person: "Persona",
};
