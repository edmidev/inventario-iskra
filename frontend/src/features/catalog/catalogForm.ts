import { z } from "zod";
import { locationKindSchema } from "../../lib/api/schemas";

export const categorySchema = z.object({
  name: z.string().trim().min(1, "Escribe el nombre.").max(120, "Máximo 120 caracteres."),
  code_prefix: z
    .string()
    .trim()
    .transform((value) => value.toUpperCase())
    .pipe(z.string().regex(/^[A-Z0-9]{2,8}$/, "Usa de 2 a 8 letras o números, sin espacios.")),
});
export type CategoryFormInput = z.input<typeof categorySchema>;
export type CategoryFormValues = z.output<typeof categorySchema>;

export const locationFormSchema = z.object({
  name: z.string().trim().min(1, "Escribe el nombre.").max(120, "Máximo 120 caracteres."),
  kind: locationKindSchema,
  parent_id: z.string(),
});
export type LocationFormValues = z.infer<typeof locationFormSchema>;
