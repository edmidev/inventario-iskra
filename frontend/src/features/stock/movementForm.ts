import { z } from "zod";
import type { MovementPayload } from "./api/stock";

export type QuantityMovement = "in" | "out" | "adjust";

export interface MovementFormValues {
  qty: string;
  note: string;
}

export const movementFieldNames = ["qty", "note"] as const;

export const movementCopy: Record<QuantityMovement, { title: string; qtyLabel: string; submit: string; pending: string; done: string }> = {
  in: { title: "Entrada de existencias", qtyLabel: "Cantidad que entra", submit: "Registrar entrada", pending: "Registrando…", done: "Entrada registrada." },
  out: { title: "Salida de existencias", qtyLabel: "Cantidad que sale", submit: "Registrar salida", pending: "Registrando…", done: "Salida registrada." },
  adjust: { title: "Ajuste de inventario", qtyLabel: "Cantidad contada", submit: "Guardar ajuste", pending: "Guardando…", done: "Ajuste registrado." },
};

export function movementFormSchema(mode: QuantityMovement) {
  const min = mode === "adjust" ? 0 : 1;
  return z.object({
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

/** Stock after applying the movement, or null when the quantity is not a valid number yet. */
export function resultingStock(mode: QuantityMovement, current: number, qty: string): number | null {
  if (!/^\d+$/.test(qty.trim())) return null;
  const amount = Number(qty);
  return mode === "in" ? current + amount : mode === "out" ? current - amount : amount;
}

export function toMovementPayload(mode: QuantityMovement, values: MovementFormValues): MovementPayload {
  const note = values.note.trim();
  return { type: mode, qty: Number(values.qty), ...(note ? { note } : {}) };
}
