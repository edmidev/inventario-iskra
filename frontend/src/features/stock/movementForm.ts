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
  const min = mode === "adjust" ? 0 : 1;
  return z.object({
    location_id: z.string().min(1, "Elige la ubicación."),
    qty: z
      .string()
      .trim()
      .refine((value) => /^\d+$/.test(value) && Number(value) >= min, min === 0 ? "Indica un número entero de 0 o más." : "Indica un número entero mayor a 0."),
    note:
      mode === "adjust"
        ? z.string().trim().min(1, "Explica el motivo del ajuste.").max(500, "Máximo 500 caracteres.")
        : z.string().max(500, "Máximo 500 caracteres."),
  });
}

/** Quantity at the location after applying the movement, or null when the quantity is not a valid number yet. */
export function resultingStock(mode: QuantityMovement, current: number, qty: string): number | null {
  if (!/^\d+$/.test(qty.trim())) return null;
  const amount = Number(qty);
  return mode === "in" ? current + amount : mode === "out" ? current - amount : amount;
}

export function toMovementPayload(mode: QuantityMovement, values: MovementFormValues): MovementPayload {
  const note = values.note.trim();
  return { type: mode, location_id: Number(values.location_id), qty: Number(values.qty), ...(note ? { note } : {}) };
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
      qty: z.string().trim().refine((value) => /^\d+$/.test(value) && Number(value) >= 1, "Indica un número entero mayor a 0."),
      note: z.string().max(500, "Máximo 500 caracteres."),
    })
    .superRefine((values, ctx) => {
      if (values.from_location_id && values.from_location_id === values.to_location_id) {
        ctx.addIssue({ code: "custom", path: ["to_location_id"], message: "El destino debe ser distinto del origen." });
      }
      const available = stockAt(stocks, values.from_location_id);
      if (values.from_location_id && /^\d+$/.test(values.qty.trim()) && Number(values.qty) > available) {
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
    qty: Number(values.qty),
    ...(note ? { note } : {}),
  };
}
