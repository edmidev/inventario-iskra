import type { Assignment, Movement } from "../../lib/api/schemas";

export type TimelineEvent =
  | { key: string; at: string; kind: "movement"; movement: Movement }
  | { key: string; at: string; kind: "assigned" | "returned"; assignment: Assignment };

export type StreamName = "movements" | "custody";

export interface TimelineInput {
  movements: Movement[];
  movementsHasMore: boolean;
  assignments: Assignment[];
  custodyHasMore: boolean;
}

export interface Timeline {
  events: TimelineEvent[];
  /** True while any stream still has older pages (or events are held back until the other stream catches up). */
  hasMore: boolean;
  /** The stream whose next page extends the visible history; null when everything is loaded. */
  nextStream: StreamName | null;
}

const time = (iso: string) => new Date(iso).getTime();

/**
 * Merges two independently paginated, newest-first streams into one timeline.
 *
 * A stream that still has older pages may hold events older than anything the other stream loaded
 * so far, so events older than its oldest loaded event are held back until that stream is fetched.
 * Without this cutoff "Cargar más" could reveal an event above ones already shown.
 */
export function mergeTimeline({ movements, movementsHasMore, assignments, custodyHasMore }: TimelineInput): Timeline {
  const all: TimelineEvent[] = movements.map((movement) => ({
    key: `m-${movement.id}`,
    at: movement.created_at,
    kind: "movement",
    movement,
  }));
  for (const assignment of assignments) {
    all.push({ key: `a-${assignment.id}`, at: assignment.assigned_at, kind: "assigned", assignment });
    if (assignment.returned_at) {
      all.push({ key: `r-${assignment.id}`, at: assignment.returned_at, kind: "returned", assignment });
    }
  }
  all.sort((a, b) => time(b.at) - time(a.at) || (a.key < b.key ? 1 : -1));

  const cutoffs: { stream: StreamName; at: number }[] = [];
  if (movementsHasMore && movements.length > 0) {
    cutoffs.push({ stream: "movements", at: Math.min(...movements.map((m) => time(m.created_at))) });
  }
  if (custodyHasMore && assignments.length > 0) {
    cutoffs.push({ stream: "custody", at: Math.min(...assignments.map((a) => time(a.assigned_at))) });
  }
  const binding = cutoffs.sort((a, b) => b.at - a.at)[0];
  const events = binding ? all.filter((event) => time(event.at) >= binding.at) : all;

  const nextStream = binding?.stream ?? (movementsHasMore ? "movements" : custodyHasMore ? "custody" : null);
  return { events, hasMore: nextStream !== null, nextStream };
}
