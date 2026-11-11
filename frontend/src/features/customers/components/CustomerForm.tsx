import { zodResolver } from "@hookform/resolvers/zod";
import type { Resolver } from "react-hook-form";
import { Input, Textarea } from "../../../components/atoms/Input";
import { FormAlert, FormFooter } from "../../../components/molecules/FormFooter";
import { FormField } from "../../../components/molecules/FormField";
import { customerFormSchema, type CustomerValues } from "../schemas";
import type { Customer } from "../schemas";
import { useApiForm } from "../../../lib/useApiForm";
interface CustomerFormProps {
  /** Present when editing. */
  customer?: Customer;
  onSubmit: (values: CustomerValues) => Promise<void>;
  onCancel: () => void;
}
export function CustomerForm({
  customer,
  onSubmit,
  onCancel
}: CustomerFormProps) {
  const {
    form,
    submit,
    formError
  } = useApiForm<CustomerValues>({
    resolver: zodResolver(customerFormSchema) as Resolver<CustomerValues>,
    defaultValues: {
      name: customer?.name ?? "",
      identification: customer?.identification ?? "",
      contact: customer?.contact ?? "",
      notes: customer?.notes ?? "",
      active: customer?.active ?? true
    },
    fields: ["name", "identification", "contact", "notes", "active"],
    codeFields: {
      customer_name_taken: "name"
    },
    onSubmit
  });
  const {
    register,
    formState
  } = form;
  const {
    errors,
    isSubmitting
  } = formState;
  return <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="Nombre" required error={errors.name?.message}>
        <Input autoComplete="off" {...register("name")} />
      </FormField>
      <FormField label="Identificación" error={errors.identification?.message}>
        <Input autoComplete="off" {...register("identification")} />
      </FormField>
      <FormField label="Contacto" error={errors.contact?.message}>
        <Input autoComplete="off" {...register("contact")} />
      </FormField>
      <FormField label="Notas" error={errors.notes?.message}>
        <Textarea rows={3} {...register("notes")} />
      </FormField>
      <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-xl border border-line px-4 py-2">
        <input type="checkbox" className="size-5 accent-(--accent)" {...register("active")} />
        <span>
          <span className="block text-sm font-semibold">Cliente activo</span>
          <span className="block text-xs text-ink-soft">Los clientes inactivos se conservan en el historial.</span>
        </span>
      </label>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel={customer ? "Guardar cambios" : "Crear cliente"} pendingLabel={customer ? "Guardando…" : "Creando…"} isSubmitting={isSubmitting} />
    </form>;
}
