import { z } from "zod";
import { roleSchema } from "../../lib/api/schemas";

export const roleLabels = { admin: "Administrador", operator: "Operador", viewer: "Solo lectura" } as const;
export const roleOptions = roleSchema.options.map((value) => ({ value, label: roleLabels[value] }));

export const MIN_PASSWORD = 10;
export const MAX_PASSWORD = 128;

export const passwordRule = z
  .string()
  .min(MIN_PASSWORD, `Usa al menos ${MIN_PASSWORD} caracteres.`)
  .max(MAX_PASSWORD, `Máximo ${MAX_PASSWORD} caracteres.`);

const nameRule = z.string().trim().min(1, "Escribe el nombre.").max(120, "Máximo 120 caracteres.");

export const createUserSchema = z.object({
  name: nameRule,
  email: z.string().trim().min(1, "Escribe el correo.").pipe(z.email("Escribe un correo válido.")).pipe(z.string().max(254, "Máximo 254 caracteres.")),
  role: roleSchema,
  password: passwordRule,
});
export type CreateUserValues = z.infer<typeof createUserSchema>;

export const editUserSchema = z.object({
  name: nameRule,
  role: roleSchema,
  active: z.boolean(),
});
export type EditUserValues = z.infer<typeof editUserSchema>;

export const resetPasswordSchema = z.object({ new_password: passwordRule });
export type ResetPasswordValues = z.infer<typeof resetPasswordSchema>;

export const changePasswordSchema = z
  .object({
    current_password: z.string().min(1, "Escribe tu contraseña actual."),
    new_password: passwordRule,
    confirm: z.string().min(1, "Repite la nueva contraseña."),
  })
  .refine((values) => values.confirm === "" || values.new_password === values.confirm, {
    path: ["confirm"],
    message: "Las contraseñas no coinciden.",
  });
export type ChangePasswordValues = z.infer<typeof changePasswordSchema>;
