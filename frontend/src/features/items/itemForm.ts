import { z } from "zod";
import type { Item } from "../../lib/api/schemas";
import type { ItemCreatePayload, ItemUpdatePayload } from "./api/items";

export interface AttributeRow {
  key: string;
  value: string;
}

export interface ItemFormValues {
  kind: "asset" | "consumable";
  code: string;
  description: string;
  category_id: string;
  location_id: string;
  status: "functional" | "inoperative" | "in_repair";
  unit: string;
  stock_current: string;
  stock_min: string;
  maintenance_interval_days: string;
  notes: string;
  attributes: AttributeRow[];
}

export const formFieldNames = [
  "kind",
  "code",
  "description",
  "category_id",
  "location_id",
  "status",
  "unit",
  "stock_current",
  "stock_min",
  "maintenance_interval_days",
  "notes",
  "attributes",
] as const;

const wholeNumber = (min: number, max?: number) => (value: string) => {
  if (value.trim() === "") return true;
  if (!/^\d+$/.test(value.trim())) return false;
  const n = Number(value);
  return n >= min && (max === undefined || n <= max);
};

export const itemFormSchema = z
  .object({
    kind: z.enum(["asset", "consumable"]),
    code: z.string(),
    description: z.string().trim().min(1, "Describe el material.").max(500, "Máximo 500 caracteres."),
    category_id: z.string().min(1, "Elige una categoría."),
    location_id: z.string(),
    status: z.enum(["functional", "inoperative", "in_repair"]),
    unit: z.string().trim().max(30, "Máximo 30 caracteres."),
    stock_current: z.string().refine(wholeNumber(0), "Usa un número entero mayor o igual a 0."),
    stock_min: z.string().refine(wholeNumber(0), "Usa un número entero mayor o igual a 0."),
    maintenance_interval_days: z.string().refine(wholeNumber(1, 3650), "Usa un número de días entre 1 y 3650."),
    notes: z.string().max(4000, "Máximo 4000 caracteres."),
    attributes: z.array(z.object({ key: z.string(), value: z.string() })),
  })
  .superRefine((values, ctx) => {
    if (values.kind === "consumable" && values.unit.trim() === "") {
      ctx.addIssue({ code: "custom", path: ["unit"], message: "Indica la unidad (por ejemplo: pza, caja)." });
    }
    const keys = values.attributes.map((row) => row.key.trim()).filter(Boolean);
    if (new Set(keys.map((k) => k.toLowerCase())).size !== keys.length) {
      ctx.addIssue({ code: "custom", path: ["attributes"], message: "Hay atributos con el mismo nombre." });
    }
    if (values.attributes.some((row) => row.value.trim() !== "" && row.key.trim() === "")) {
      ctx.addIssue({ code: "custom", path: ["attributes"], message: "Cada atributo necesita un nombre." });
    }
  });

export const emptyItemForm: ItemFormValues = {
  kind: "asset",
  code: "",
  description: "",
  category_id: "",
  location_id: "",
  status: "functional",
  unit: "",
  stock_current: "0",
  stock_min: "0",
  maintenance_interval_days: "",
  notes: "",
  attributes: [],
};

function attributeValueToText(value: unknown): string {
  if (typeof value === "string") return value;
  if (value === null || value === undefined) return "";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}

export function itemToFormValues(item: Item): ItemFormValues {
  return {
    kind: item.kind,
    code: item.code,
    description: item.description,
    category_id: String(item.category_id),
    location_id: item.location_id === null ? "" : String(item.location_id),
    status: item.status,
    unit: item.unit ?? "",
    stock_current: String(item.stock_current),
    stock_min: String(item.stock_min),
    maintenance_interval_days: item.maintenance_interval_days === null ? "" : String(item.maintenance_interval_days),
    notes: item.notes,
    attributes: Object.entries(item.attributes).map(([key, value]) => ({ key, value: attributeValueToText(value) })),
  };
}

function attributesToObject(rows: AttributeRow[]): Record<string, string> {
  const result: Record<string, string> = {};
  for (const row of rows) {
    const key = row.key.trim();
    if (key) result[key] = row.value.trim();
  }
  return result;
}

export function toCreatePayload(values: ItemFormValues, isAdmin: boolean): ItemCreatePayload {
  const payload: ItemCreatePayload = {
    kind: values.kind,
    description: values.description.trim(),
    category_id: Number(values.category_id),
    notes: values.notes,
  };
  if (isAdmin && values.code.trim()) payload.code = values.code.trim().toUpperCase();
  if (values.location_id) payload.location_id = Number(values.location_id);
  const attributes = attributesToObject(values.attributes);
  if (Object.keys(attributes).length > 0) payload.attributes = attributes;
  if (values.kind === "consumable") {
    payload.unit = values.unit.trim();
    payload.stock_current = Number(values.stock_current || "0");
    payload.stock_min = Number(values.stock_min || "0");
  } else if (values.maintenance_interval_days.trim()) {
    payload.maintenance_interval_days = Number(values.maintenance_interval_days);
  }
  return payload;
}

/** Builds a PATCH body. Location and attributes are sent only when they changed. */
export function toUpdatePayload(values: ItemFormValues, initial: ItemFormValues): ItemUpdatePayload {
  const payload: ItemUpdatePayload = {
    description: values.description.trim(),
    category_id: Number(values.category_id),
    status: values.status,
    notes: values.notes,
  };
  if (values.location_id !== initial.location_id) {
    payload.location_id = values.location_id ? Number(values.location_id) : null;
  }
  if (JSON.stringify(attributesToObject(values.attributes)) !== JSON.stringify(attributesToObject(initial.attributes))) {
    payload.attributes = attributesToObject(values.attributes);
  }
  if (values.kind === "consumable") {
    payload.unit = values.unit.trim();
    payload.stock_min = Number(values.stock_min || "0");
  } else {
    payload.maintenance_interval_days = values.maintenance_interval_days.trim()
      ? Number(values.maintenance_interval_days)
      : null;
  }
  return payload;
}
