import type { ReorderGroup } from "../../lib/api/schemas";
import { emptyLine, emptyOrder, type OrderValues } from "./orderForm";
import { minorToInput } from "./money";

/**
 * Prefills a draft from one suggestion group. The order has a single currency:
 * the first line's last currency wins, and a line last bought in another
 * currency keeps an empty cost so the user types it.
 */
export function groupToOrderValues(group: ReorderGroup): OrderValues {
  const currency = group.lines.find((line) => line.last_currency)?.last_currency ?? "USD";
  return {
    ...emptyOrder(),
    supplier_id: group.supplier_id === null ? "" : String(group.supplier_id),
    currency,
    lines: group.lines.map((line) => ({
      ...emptyLine(),
      item_id: line.item_id,
      item_label: `${line.code} · ${line.description}`,
      quantity: String(line.suggested_qty),
      unit_cost: line.last_unit_cost_minor !== null && line.last_currency === currency ? minorToInput(line.last_unit_cost_minor) : "",
    })),
  };
}
