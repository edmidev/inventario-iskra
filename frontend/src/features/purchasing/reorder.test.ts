import { describe, expect, it } from "vitest";
import type { ReorderGroup, ReorderLine } from "../../lib/api/schemas";
import { groupToOrderValues } from "./reorder";

const line = (overrides: Partial<ReorderLine> = {}): ReorderLine => ({
  item_id: "i1",
  code: "ISK-CON-001",
  description: "Cinta",
  unit: "rollo",
  stock_current: 1,
  stock_min: 5,
  pending_ordered: 0,
  suggested_qty: 9,
  last_unit_cost_minor: 1250,
  last_currency: "USD",
  ...overrides,
});

describe("groupToOrderValues", () => {
  it("prefills supplier, currency, quantity and cost in decimal text", () => {
    const group: ReorderGroup = { supplier_id: 7, supplier_name: "Alfa", lines: [line()] };
    const values = groupToOrderValues(group);
    expect(values.supplier_id).toBe("7");
    expect(values.currency).toBe("USD");
    expect(values.lines).toHaveLength(1);
    expect(values.lines[0]).toMatchObject({ source: "existing", item_id: "i1", quantity: "9", unit_cost: "12.50" });
  });

  it("leaves the supplier empty for the unassigned group and defaults to USD", () => {
    const values = groupToOrderValues({
      supplier_id: null,
      supplier_name: null,
      lines: [line({ last_unit_cost_minor: null, last_currency: null })],
    });
    expect(values.supplier_id).toBe("");
    expect(values.currency).toBe("USD");
    expect(values.lines[0].unit_cost).toBe("");
  });

  it("keeps one currency and blanks the cost of a line bought in the other", () => {
    const values = groupToOrderValues({
      supplier_id: 2,
      supplier_name: "Beta",
      lines: [line({ last_currency: "VES", last_unit_cost_minor: 5000 }), line({ item_id: "i2", last_currency: "USD", last_unit_cost_minor: 100 })],
    });
    expect(values.currency).toBe("VES");
    expect(values.lines.map((l) => l.unit_cost)).toEqual(["50.00", ""]);
  });
});
