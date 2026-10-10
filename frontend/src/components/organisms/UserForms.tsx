import { zodResolver } from "@hookform/resolvers/zod";
import type { Resolver } from "react-hook-form";
import { Button } from "../atoms/Button";
import { Input } from "../atoms/Input";
import { ControlledSelect } from "../molecules/ControlledSelect";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import {
  changePasswordSchema,
  createUserSchema,
  editUserSchema,
  resetPasswordSchema,
  roleOptions,
  type ChangePasswordValues,
  type CreateUserValues,
  type EditUserValues,
  type ResetPasswordValues,
} from "../../features/users/userForm";
import { useApiForm } from "../../lib/useApiForm";
import type { User } from "../../lib/api/schemas";

interface BaseProps<T> {
  onSubmit: (values: T) => Promise<void>;
  onCancel: () => void;
}

export function CreateUserForm({ onSubmit, onCancel }: BaseProps<CreateUserValues>) {
  const { form, submit, formError } = useApiForm<CreateUserValues>({
    resolver: zodResolver(createUserSchema) as Resolver<CreateUserValues>,
    defaultValues: { name: "", email: "", role: "operator", password: "" },
    fields: ["name", "email", "role", "password"],
    codeFields: { email_taken: "email" },
    onSubmit,
  });
  const { register, control, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="Nombre" required error={errors.name?.message}>
        <Input autoComplete="off" {...register("name")} />
      </FormField>
      <FormField label="Correo" required error={errors.email?.message}>
        <Input type="email" inputMode="email" autoComplete="off" {...register("email")} />
      </FormField>
      <ControlledSelect control={control} name="role" label="Rol" placeholder="Elige un rol" options={roleOptions} required error={errors.role?.message} />
      <FormField label="Contraseña inicial" required hint="Mínimo 10 caracteres. La persona podrá cambiarla en Mi cuenta." error={errors.password?.message}>
        <Input type="password" autoComplete="new-password" {...register("password")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Crear usuario" pendingLabel="Creando…" isSubmitting={isSubmitting} />
    </form>
  );
}

export function EditUserForm({ user, onSubmit, onCancel }: BaseProps<EditUserValues> & { user: User }) {
  const { form, submit, formError } = useApiForm<EditUserValues>({
    resolver: zodResolver(editUserSchema) as Resolver<EditUserValues>,
    defaultValues: { name: user.name, role: user.role, active: user.active },
    fields: ["name", "role", "active"],
    onSubmit,
  });
  const { register, control, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <p className="rounded-xl bg-sunken px-4 py-3 text-sm break-all text-ink-soft">{user.email}</p>
      <FormField label="Nombre" required error={errors.name?.message}>
        <Input autoComplete="off" {...register("name")} />
      </FormField>
      <ControlledSelect control={control} name="role" label="Rol" placeholder="Elige un rol" options={roleOptions} required error={errors.role?.message} />
      <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-xl border border-line px-4 py-2">
        <input type="checkbox" className="size-5 accent-(--accent)" {...register("active")} />
        <span>
          <span className="block text-sm font-semibold">Cuenta activa</span>
          <span className="block text-xs text-ink-soft">Si la desactivas, se cierran sus sesiones y no podrá entrar.</span>
        </span>
      </label>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Guardar cambios" pendingLabel="Guardando…" isSubmitting={isSubmitting} />
    </form>
  );
}

export function ResetPasswordForm({ onSubmit, onCancel }: BaseProps<ResetPasswordValues>) {
  const { form, submit, formError } = useApiForm<ResetPasswordValues>({
    resolver: zodResolver(resetPasswordSchema) as Resolver<ResetPasswordValues>,
    defaultValues: { new_password: "" },
    fields: ["new_password"],
    onSubmit,
  });
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="Nueva contraseña" required hint="Mínimo 10 caracteres. Se cierran las sesiones de esta persona." error={errors.new_password?.message}>
        <Input type="password" autoComplete="new-password" {...register("new_password")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Restablecer" pendingLabel="Guardando…" isSubmitting={isSubmitting} danger />
    </form>
  );
}

export function ChangePasswordForm({ onSubmit }: { onSubmit: (values: ChangePasswordValues) => Promise<void> }) {
  const { form, submit, formError } = useApiForm<ChangePasswordValues>({
    resolver: zodResolver(changePasswordSchema) as Resolver<ChangePasswordValues>,
    defaultValues: { current_password: "", new_password: "", confirm: "" },
    fields: ["current_password", "new_password"],
    codeFields: { invalid_current_password: "current_password" },
    onSubmit,
  });
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate aria-label="Cambiar contraseña" className="flex flex-col gap-4">
      <FormField label="Contraseña actual" required error={errors.current_password?.message}>
        <Input type="password" autoComplete="current-password" {...register("current_password")} />
      </FormField>
      <FormField label="Nueva contraseña" required hint="Mínimo 10 caracteres." error={errors.new_password?.message}>
        <Input type="password" autoComplete="new-password" {...register("new_password")} />
      </FormField>
      <FormField label="Repite la nueva contraseña" required error={errors.confirm?.message}>
        <Input type="password" autoComplete="new-password" {...register("confirm")} />
      </FormField>
      <FormAlert message={formError} />
      <Button type="submit" variant="primary" disabled={isSubmitting}>
        {isSubmitting ? "Guardando…" : "Cambiar contraseña"}
      </Button>
    </form>
  );
}
