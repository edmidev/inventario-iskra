import type { Location } from "../../lib/api/schemas";

/** "Parent › Child" label for a location id, or null when unknown. */
export function locationPath(locations: Location[] | undefined, id: number | null): string | null {
  if (id === null || !locations) return null;
  const byId = new Map(locations.map((l) => [l.id, l]));
  const parts: string[] = [];
  const seen = new Set<number>();
  let current = byId.get(id);
  while (current && !seen.has(current.id)) {
    seen.add(current.id);
    parts.unshift(current.name);
    current = current.parent_id === null ? undefined : byId.get(current.parent_id);
  }
  return parts.length ? parts.join(" › ") : null;
}

