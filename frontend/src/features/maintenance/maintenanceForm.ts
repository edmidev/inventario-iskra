import { z } from "zod";
import { toDateInput, toDateTimeInput } from "../../lib/dates";

const optionalMoney = z
  .string()
  .trim()
  .refine((value) => value === "" || /^\d+([.,]\d{1,2})?$/.test(value), "Usa un monto válido, por ejemplo 150 o 150.50.");

export const parseMoney = (value: string): number | undefined => (value.trim() === "" ? undefined : Number(value.trim().replace(",", ".")));

const notInFuture = (value: string) => value !== "" && value <= toDateTimeInput();

export const reportFailureSchema = z.object({
  description: z.string().trim().min(1, "Describe la falla.").max(1000, "Máximo 1000 caracteres."),
  destination: z.string().trim().max(120, "Máximo 120 caracteres."),
  item_status: z.enum(["inoperative", "in_repair"]),
  reported_at: z.string().refine(notInFuture, "La fecha de la falla no puede estar en el futuro."),
});
export type ReportFailureValues = z.infer<typeof reportFailureSchema>;

export const updateFailureSchema = z.object({
  description: z.string().trim().min(1, "Describe la falla.").max(1000, "Máximo 1000 caracteres."),
  destination: z.string().trim().max(120, "Máximo 120 caracteres."),
  item_status: z.enum(["inoperative", "in_repair"]),
});
export type UpdateFailureValues = z.infer<typeof updateFailureSchema>;

export const closeFailureSchema = z.object({
  cost: optionalMoney,
  resolution_note: z.string().trim().max(1000, "Máximo 1000 caracteres."),
  item_status_after: z.enum(["functional", "inoperative"]),
});
export type CloseFailureValues = z.infer<typeof closeFailureSchema>;

export const preventiveSchema = z
  .object({
    performed_at: z.string().refine(notInFuture, "La fecha no puede estar en el futuro."),
    description: z.string().trim().min(1, "Describe qué se hizo.").max(1000, "Máximo 1000 caracteres."),
    cost: optionalMoney,
    next_due_at: z.string(),
  })
  .superRefine((values, ctx) => {
    if (values.next_due_at && values.next_due_at <= values.performed_at.slice(0, 10)) {
      ctx.addIssue({ code: "custom", path: ["next_due_at"], message: "El próximo mantenimiento debe ser posterior a la fecha realizada." });
    }
  });
export type PreventiveValues = z.infer<typeof preventiveSchema>;

export const nowInput = () => toDateTimeInput();
export const todayInput = () => toDateInput();
