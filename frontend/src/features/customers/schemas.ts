import { z } from "zod";
export const customerSchema = z.object({
  id: z.number(),
  name: z.string(),
  identification: z.string(),
  contact: z.string(),
  notes: z.string(),
  active: z.boolean(),
  created_at: z.string(),
  updated_at: z.string()
});
export type Customer = z.infer<typeof customerSchema>;
export const customerFormSchema = z.object({
  name: z.string().trim().min(1, "Escribe el nombre.").max(120),
  identification: z.string().trim().max(120),
  contact: z.string().trim().max(254),
  notes: z.string().trim().max(1000),
  active: z.boolean()
});
export type CustomerValues = z.infer<typeof customerFormSchema>;
