import { z } from "zod";
import { ApiError } from "../../lib/api/errors";
import type { PurchaseOrder } from "../../lib/api/schemas";
import type { LinePayload, OrderPayload } from "./api/purchaseOrders";
import { minorToInput, parseMinorUnits } from "./money";

export const MAX_ASSET_UNITS = 100;
export const MAX_LINE_QUANTITY = 100000;

export const lineSchema = z
  .object({
    source: z.enum(["existing", "new"]),
    item_id: z.string(),
    /** Display only: what the picked item is called. */
    item_label: z.string(),
    description: z.string(),
    category_id: z.string(),
    kind: z.enum(["asset", "consumable"]),
    unit: z.string(),
    stock_min: z.string(),
    quantity: z.string(),
    unit_cost: z.string(),
  })
  .superRefine((line, ctx) => {
    const issue = (path: string, message: string) => ctx.addIssue({ code: "custom", path: [path], message });
    if (line.source === "existing") {
      if (!line.item_id) issue("item_id", "Elige un consumible.");
    } else {
      const description = line.description.trim();
      if (!description) issue("description", "Describe el ítem.");
      else if (description.length > 500) issue("description", "Máximo 500 caracteres.");
      if (!line.category_id) issue("category_id", "Elige una categoría.");
      if (line.kind === "consumable") {
        if (!line.unit.trim()) issue("unit", "Indica la unidad (caja, litro…).");
        else if (line.unit.trim().length > 30) issue("unit", "Máximo 30 caracteres.");
        if (line.stock_min.trim() !== "" && !/^\d+$/.test(line.stock_min.trim())) issue("stock_min", "Usa un número entero, 0 o más.");
      }
    }
    const quantity = parseQuantity(line.quantity);
    const isNewAsset = line.source === "new" && line.kind === "asset";
    if (quantity === null) issue("quantity", "Indica una cantidad entera de 1 o más.");
    else if (quantity > MAX_LINE_QUANTITY) issue("quantity", `Máximo ${MAX_LINE_QUANTITY} por línea.`);
    else if (isNewAsset && quantity > MAX_ASSET_UNITS) issue("quantity", `Máximo ${MAX_ASSET_UNITS} ítems por línea de activos.`);
    if (parseMinorUnits(line.unit_cost) === null) issue("unit_cost", "Usa un monto válido, por ejemplo 150 o 150.50.");
  });
export type LineValues = z.infer<typeof lineSchema>;

export const orderSchema = z.object({
  supplier_id: z.string().min(1, "Elige un proveedor."),
  currency: z.enum(["USD", "VES"]),
  exchange_rate: z
    .string()
    .trim()
    .refine((value) => value === "" || (/^\d+([.,]\d+)?$/.test(value) && Number(value.replace(",", ".")) > 0), "Usa una tasa válida, por ejemplo 36.50."),
  expected_at: z.string(),
  notes: z.string().trim().max(1000, "Máximo 1000 caracteres."),
  lines: z.array(lineSchema).max(200, "Máximo 200 líneas."),
});
export type OrderValues = z.infer<typeof orderSchema>;

export const parseQuantity = (text: string): number | null => (/^\d+$/.test(text.trim()) && Number(text) >= 1 ? Number(text) : null);

export const emptyLine = (): LineValues => ({
  source: "existing",
  item_id: "",
  item_label: "",
  description: "",
  category_id: "",
  kind: "consumable",
  unit: "",
  stock_min: "",
  quantity: "1",
  unit_cost: "",
});

export const emptyOrder = (): OrderValues => ({ supplier_id: "", currency: "USD", exchange_rate: "", expected_at: "", notes: "", lines: [] });

export function orderToValues(order: PurchaseOrder): OrderValues {
  return {
    supplier_id: String(order.supplier_id),
    currency: order.currency,
    exchange_rate: order.exchange_rate === null ? "" : String(order.exchange_rate),
    expected_at: order.expected_at?.slice(0, 10) ?? "",
    notes: order.notes,
    lines: order.lines.map((line) => ({
      ...emptyLine(),
      source: line.new_item ? "new" : "existing",
      item_id: line.item_id ?? "",
      item_label: line.description,
      description: line.new_item?.description ?? "",
      category_id: line.new_item ? String(line.new_item.category_id) : "",
      kind: line.new_item?.kind ?? "consumable",
      unit: line.new_item?.unit ?? "",
      stock_min: line.new_item?.stock_min ? String(line.new_item.stock_min) : "",
      quantity: String(line.quantity),
      unit_cost: minorToInput(line.unit_cost_minor),
    })),
  };
}

function lineToPayload(line: LineValues): LinePayload {
  const base = { quantity: Number(line.quantity), unit_cost_minor: parseMinorUnits(line.unit_cost) ?? 0 };
  if (line.source === "existing") return { item_id: line.item_id, ...base };
  const stockMin = line.stock_min.trim();
  return {
    new_item: {
      description: line.description.trim(),
      category_id: Number(line.category_id),
      kind: line.kind,
      ...(line.kind === "consumable" ? { unit: line.unit.trim(), ...(stockMin ? { stock_min: Number(stockMin) } : {}) } : {}),
    },
    ...base,
  };
}

export function valuesToPayload(values: OrderValues): OrderPayload {
  const rate = values.exchange_rate.trim();
  return {
    supplier_id: Number(values.supplier_id),
    currency: values.currency,
    exchange_rate: rate ? Number(rate.replace(",", ".")) : null,
    expected_at: values.expected_at || null,
    notes: values.notes,
    lines: values.lines.map(lineToPayload),
  };
}

/** Sum of the lines that already parse, in minor units. */
export function linesTotalMinor(lines: LineValues[]): number {
  return lines.reduce((sum, line) => {
    const quantity = parseQuantity(line.quantity);
    const cost = parseMinorUnits(line.unit_cost);
    return quantity === null || cost === null ? sum : sum + quantity * cost;
  }, 0);
}

/** Re-keys API field errors so they land on the form inputs (`lines[0].new_item.unit` -> `lines[0].unit`). */
export function remapFields(error: unknown, remap: (field: string) => string): unknown {
  if (!(error instanceof ApiError) || Object.keys(error.fields).length === 0) return error;
  const fields = Object.fromEntries(Object.entries(error.fields).map(([field, message]) => [remap(field), message]));
  return new ApiError(error.status, error.code, error.message, fields);
}

export const orderFieldToForm = (field: string) =>
  field.replace(/^lines\[(\d+)\]\.new_item\./, "lines[$1].").replace(/^lines\[(\d+)\]\.unit_cost_minor$/, "lines[$1].unit_cost");
