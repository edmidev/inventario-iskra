import { parseQuantity, quantityAfter } from "../../lib/quantity";
import { z } from "zod";
import type { ItemStock } from "../../lib/api/schemas";
import type { MovementPayload } from "./api/stock";
import { stockAt } from "./stocks";

export type QuantityMovement = "in" | "out" | "adjust";

export interface MovementFormValues {
  location_id: string;
  qty: string;
  note: string;
}

export const movementFieldNames = ["location_id", "qty", "note"] as const;

export const movementCopy: Record<QuantityMovement, { title: string; qtyLabel: string; submit: string; pending: string; done: string }> = {
  in: { title: "Entrada de existencias", qtyLabel: "Cantidad que entra", submit: "Registrar entrada", pending: "Registrando…", done: "Entrada registrada" },
  out: { title: "Salida de existencias", qtyLabel: "Cantidad que sale", submit: "Registrar salida", pending: "Registrando…", done: "Salida registrada" },
  adjust: { title: "Ajuste de inventario", qtyLabel: "Cantidad contada", submit: "Guardar ajuste", pending: "Guardando…", done: "Ajuste registrado" },
};

export function movementFormSchema(mode: QuantityMovement) {
  const min = mode === "adjust" ? 0 : 0.001;
  return z.object({
    location_id: z.string().min(1, "Elige la ubicación."),
    qty: z
      .string()
      .trim()
      .refine((value) => parseQuantity(value) !== null && (parseQuantity(value) ?? 0) >= min, min === 0 ? "Indica una cantidad de 0 o más, hasta 3 decimales." : "Indica una cantidad mayor a 0, hasta 3 decimales."),
    note:
      mode === "adjust"
        ? z.string().trim().min(1, "Explica el motivo del ajuste.").max(500, "Máximo 500 caracteres.")
        : z.string().max(500, "Máximo 500 caracteres."),
  });
}

/** Quantity at the location after applying the movement, or null when the quantity is not a valid number yet. */
export function resultingStock(mode: QuantityMovement, current: number, qty: string): number | null {
  const amount = parseQuantity(qty);
  if (amount === null) return null;
  return mode === "in" ? quantityAfter(current, amount) : mode === "out" ? quantityAfter(current, -amount) : amount;
}

export function toMovementPayload(mode: QuantityMovement, values: MovementFormValues): MovementPayload {
  const note = values.note.trim();
  return { type: mode, location_id: Number(values.location_id), qty: parseQuantity(values.qty) ?? 0, ...(note ? { note } : {}) };
}

export interface TransferFormValues {
  from_location_id: string;
  to_location_id: string;
  qty: string;
  note: string;
}

export const transferFieldNames = ["from_location_id", "to_location_id", "qty", "note"] as const;

export function stockTransferSchema(stocks: readonly ItemStock[]) {
  return z
    .object({
      from_location_id: z.string().min(1, "Elige la ubicación de origen."),
      to_location_id: z.string().min(1, "Elige la ubicación de destino."),
      qty: z.string().trim().refine((value) => parseQuantity(value) !== null && (parseQuantity(value) ?? 0) > 0, "Indica una cantidad mayor a 0, hasta 3 decimales."),
      note: z.string().max(500, "Máximo 500 caracteres."),
    })
    .superRefine((values, ctx) => {
      if (values.from_location_id && values.from_location_id === values.to_location_id) {
        ctx.addIssue({ code: "custom", path: ["to_location_id"], message: "El destino debe ser distinto del origen." });
      }
      const available = stockAt(stocks, values.from_location_id);
      if (values.from_location_id && parseQuantity(values.qty) !== null && (parseQuantity(values.qty) ?? 0) > available) {
        ctx.addIssue({ code: "custom", path: ["qty"], message: `El origen solo tiene ${available}.` });
      }
    });
}

export function toStockTransferPayload(values: TransferFormValues): MovementPayload {
  const note = values.note.trim();
  return {
    type: "transfer",
    from_location_id: Number(values.from_location_id),
    to_location_id: Number(values.to_location_id),
    qty: parseQuantity(values.qty) ?? 0,
    ...(note ? { note } : {}),
  };
}
