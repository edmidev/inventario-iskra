import { describe, expect, it } from "vitest";
import type { Location } from "../../lib/api/schemas";
import { buildLocationRows, descendantIds } from "./locationTree";

const loc = (id: number, name: string, parent_id: number | null): Location => ({ id, name, kind: "storage", parent_id });

describe("location tree", () => {
  const all = [loc(3, "Estante B", 1), loc(1, "Bodega", null), loc(2, "Estante A", 1), loc(4, "Caja 1", 2), loc(5, "Ana", null)];

  it("orders depth-first with siblings by name and reports depth", () => {
    expect(buildLocationRows(all).map((r) => `${r.depth}:${r.location.name}`)).toEqual([
      "0:Ana",
      "0:Bodega",
      "1:Estante A",
      "2:Caja 1",
      "1:Estante B",
    ]);
  });

  it("never hides a location whose parent is missing", () => {
    expect(buildLocationRows([loc(9, "Huérfana", 99)]).map((r) => r.location.name)).toEqual(["Huérfana"]);
  });

  it("collects a location and all of its descendants", () => {
    expect([...descendantIds(all, 1)].sort()).toEqual([1, 2, 3, 4]);
    expect([...descendantIds(all, 5)]).toEqual([5]);
  });
});
