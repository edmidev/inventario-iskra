import { describe, expect, it } from "vitest";
import { activeFilterCount, defaultFilters, parseFilters, toSearchParams } from "./filters";

describe("item filters <-> URL", () => {
  it("round-trips every filter", () => {
    const filters = {
      ...defaultFilters,
      q: "taladro",
      categoryId: 3,
      locationId: 11,
      status: "in_repair" as const,
      kind: "asset" as const,
      availability: "unavailable" as const,
      lowStock: true,
      sort: "updated_at" as const,
      order: "desc" as const,
    };
    expect(parseFilters(toSearchParams(filters))).toEqual(filters);
  });

  it("omits defaults so URLs stay clean", () => {
    expect(toSearchParams(defaultFilters).toString()).toBe("");
  });

  it("ignores invalid values", () => {
    const parsed = parseFilters(new URLSearchParams("status=bogus&category_id=abc&sort=hack&order=up&kind=x"));
    expect(parsed).toEqual(defaultFilters);
  });

  it("counts only structural filters, not search or sort", () => {
    expect(activeFilterCount({ ...defaultFilters, q: "x", sort: "description" })).toBe(0);
    expect(activeFilterCount({ ...defaultFilters, lowStock: true, kind: "asset" })).toBe(2);
  });
});
