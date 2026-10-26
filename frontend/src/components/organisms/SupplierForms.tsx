import { zodResolver } from "@hookform/resolvers/zod";
import type { Resolver } from "react-hook-form";
import { Input, Textarea } from "../atoms/Input";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import { supplierSchema, type SupplierValues } from "../../features/suppliers/supplierForm";
import type { Supplier } from "../../lib/api/schemas";
import { useApiForm } from "../../lib/useApiForm";

interface SupplierFormProps {
  /** Present when editing. */
  supplier?: Supplier;
  onSubmit: (values: SupplierValues) => Promise<void>;
  onCancel: () => void;
}

export function SupplierForm({ supplier, onSubmit, onCancel }: SupplierFormProps) {
  const { form, submit, formError } = useApiForm<SupplierValues>({
    resolver: zodResolver(supplierSchema) as Resolver<SupplierValues>,
    defaultValues: {
      name: supplier?.name ?? "",
      contact_name: supplier?.contact_name ?? "",
      email: supplier?.email ?? "",
      phone: supplier?.phone ?? "",
      notes: supplier?.notes ?? "",
      active: supplier?.active ?? true,
    },
    fields: ["name", "contact_name", "email", "phone", "notes", "active"],
    codeFields: { supplier_name_taken: "name" },
    onSubmit,
  });
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="Nombre" required error={errors.name?.message}>
        <Input autoComplete="off" {...register("name")} />
      </FormField>
      <FormField label="Contacto" error={errors.contact_name?.message}>
        <Input autoComplete="off" {...register("contact_name")} />
      </FormField>
      <FormField label="Correo" error={errors.email?.message}>
        <Input type="email" inputMode="email" autoComplete="off" {...register("email")} />
      </FormField>
      <FormField label="Teléfono" error={errors.phone?.message}>
        <Input type="tel" inputMode="tel" autoComplete="off" {...register("phone")} />
      </FormField>
      <FormField label="Notas" error={errors.notes?.message}>
        <Textarea rows={3} {...register("notes")} />
      </FormField>
      <label className="flex min-h-11 cursor-pointer items-center gap-3 rounded-xl border border-line px-4 py-2">
        <input type="checkbox" className="size-5 accent-(--accent)" {...register("active")} />
        <span>
          <span className="block text-sm font-semibold">Proveedor activo</span>
          <span className="block text-xs text-ink-soft">Si lo desactivas no podrá usarse en órdenes nuevas.</span>
        </span>
      </label>
      <FormAlert message={formError} />
      <FormFooter
        onCancel={onCancel}
        submitLabel={supplier ? "Guardar cambios" : "Crear proveedor"}
        pendingLabel={supplier ? "Guardando…" : "Creando…"}
        isSubmitting={isSubmitting}
      />
    </form>
  );
}
