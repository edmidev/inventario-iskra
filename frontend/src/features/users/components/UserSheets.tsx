import { CreateUserForm, EditUserForm, ResetPasswordForm } from "../../../components/organisms/UserForms";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import type { User } from "../../../lib/api/schemas";
import { useCreateUser, useResetPassword, useUpdateUser, type UserUpdatePayload } from "../api/users";

interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateUserSheet({ open, onOpenChange }: SheetProps) {
  const create = useCreateUser();
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Nuevo usuario" description="La persona entrará con su correo y esta contraseña.">
      <CreateUserForm
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const saved = await create.mutateAsync({ ...values, name: values.name.trim(), email: values.email.trim() });
          toast.success(`Usuario ${saved.name} creado.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function EditUserSheet({ user, open, onOpenChange }: SheetProps & { user: User }) {
  const update = useUpdateUser(user.id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Editar usuario" description={user.email}>
      <EditUserForm
        user={user}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const payload: UserUpdatePayload = {};
          if (values.name.trim() !== user.name) payload.name = values.name.trim();
          if (values.role !== user.role) payload.role = values.role;
          if (values.active !== user.active) payload.active = values.active;
          if (Object.keys(payload).length > 0) await update.mutateAsync(payload);
          toast.success("Usuario actualizado.");
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function ResetPasswordSheet({ user, open, onOpenChange }: SheetProps & { user: User }) {
  const reset = useResetPassword(user.id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Restablecer contraseña" description={`${user.name} · ${user.email}`}>
      <ResetPasswordForm
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          await reset.mutateAsync(values.new_password);
          toast.success(`Contraseña de ${user.name} restablecida.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}
