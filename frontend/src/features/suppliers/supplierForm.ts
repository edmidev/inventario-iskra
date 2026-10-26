import { z } from "zod";

export const supplierSchema = z.object({
  name: z.string().trim().min(1, "Escribe el nombre.").max(120, "Máximo 120 caracteres."),
  contact_name: z.string().trim().max(120, "Máximo 120 caracteres."),
  email: z
    .string()
    .trim()
    .max(254, "Máximo 254 caracteres.")
    .refine((value) => value === "" || z.email().safeParse(value).success, "Escribe un correo válido."),
  phone: z.string().trim().max(40, "Máximo 40 caracteres."),
  notes: z.string().trim().max(1000, "Máximo 1000 caracteres."),
  active: z.boolean(),
});
export type SupplierValues = z.infer<typeof supplierSchema>;
