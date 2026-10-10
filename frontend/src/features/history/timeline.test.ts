import { describe, expect, it } from "vitest";
import { assignment, movement } from "../../test/fixtures";
import { mergeTimeline } from "./timeline";

const none = { movements: [], movementsHasMore: false, assignments: [], custodyHasMore: false };

describe("mergeTimeline", () => {
  it("orders movements and custody events newest first, splitting a returned loan in two events", () => {
    const timeline = mergeTimeline({
      ...none,
      movements: [
        movement({ id: 2, created_at: "2026-10-05T00:00:00Z" }),
        movement({ id: 1, created_at: "2026-08-01T00:00:00Z" }),
      ],
      assignments: [assignment({ id: 7, assigned_at: "2026-09-01T00:00:00Z", returned_at: "2026-09-10T00:00:00Z" })],
    });
    expect(timeline.events.map((e) => e.key)).toEqual(["m-2", "r-7", "a-7", "m-1"]);
    expect(timeline.hasMore).toBe(false);
    expect(timeline.nextStream).toBeNull();
  });

  it("holds back events older than the oldest loaded event of a stream with more pages", () => {
    const timeline = mergeTimeline({
      movements: [movement({ id: 3, created_at: "2026-10-05T00:00:00Z" }), movement({ id: 2, created_at: "2026-10-04T00:00:00Z" })],
      movementsHasMore: true,
      assignments: [assignment({ id: 9, assigned_at: "2026-09-01T00:00:00Z" })],
      custodyHasMore: false,
    });
    // Older movements may still exist, so the September loan cannot be shown above them yet.
    expect(timeline.events.map((e) => e.key)).toEqual(["m-3", "m-2"]);
    expect(timeline.hasMore).toBe(true);
    expect(timeline.nextStream).toBe("movements");
  });

  it("asks for the stream that limits the visible history when both have more pages", () => {
    const timeline = mergeTimeline({
      movements: [movement({ id: 3, created_at: "2026-10-05T00:00:00Z" })],
      movementsHasMore: true,
      assignments: [assignment({ id: 4, assigned_at: "2026-10-01T00:00:00Z" })],
      custodyHasMore: true,
    });
    expect(timeline.nextStream).toBe("movements");
    expect(timeline.events.map((e) => e.key)).toEqual(["m-3"]);
  });
});
