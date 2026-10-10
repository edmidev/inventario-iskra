import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Input, Textarea } from "../atoms/Input";
import { ControlledSelect } from "../molecules/ControlledSelect";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import { locationPath } from "../../features/catalog/locationPath";
import type { Location } from "../../lib/api/schemas";
import { toDateInput } from "../../lib/dates";
import { applyApiError } from "../../lib/forms";

export const assigneeKinds: Location["kind"][] = ["person", "container"];

const assignSchema = z
  .object({
    assignee_location_id: z.string().min(1, "Elige a quién se presta."),
    due_at: z.string(),
    note: z.string().max(500, "Máximo 500 caracteres."),
  })
  .superRefine((values, ctx) => {
    if (values.due_at && values.due_at < toDateInput()) {
      ctx.addIssue({ code: "custom", path: ["due_at"], message: "La fecha de devolución no puede ser anterior a hoy." });
    }
  });
export type AssignFormValues = z.infer<typeof assignSchema>;

interface AssignFormProps {
  locations: Location[];
  onSubmit: (values: AssignFormValues) => Promise<void>;
  onCancel: () => void;
}

export function AssignForm({ locations, onSubmit, onCancel }: AssignFormProps) {
  const [formError, setFormError] = useState<string | null>(null);
  const {
    register,
    control,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<AssignFormValues>({
    resolver: zodResolver(assignSchema),
    defaultValues: { assignee_location_id: "", due_at: "", note: "" },
  });

  const submit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      setFormError(applyApiError(error, setError, ["assignee_location_id", "due_at", "note"]));
    }
  });

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <ControlledSelect
        control={control}
        name="assignee_location_id"
        label="Prestar a"
        required
        error={errors.assignee_location_id?.message}
        placeholder="Persona o contenedor"
        options={locations
          .filter((location) => assigneeKinds.includes(location.kind))
          .map((location) => ({ value: String(location.id), label: locationPath(locations, location.id) ?? location.name }))}
      />
      <FormField label="Fecha de devolución" hint="Opcional." error={errors.due_at?.message}>
        <Input type="date" min={toDateInput()} {...register("due_at")} />
      </FormField>
      <FormField label="Nota" hint="Opcional." error={errors.note?.message}>
        <Textarea rows={2} {...register("note")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Registrar préstamo" pendingLabel="Registrando…" isSubmitting={isSubmitting} />
    </form>
  );
}

const returnSchema = z.object({
  return_location_id: z.string(),
  note: z.string().max(500, "Máximo 500 caracteres."),
});
export type ReturnFormValues = z.infer<typeof returnSchema>;

interface ReturnFormProps {
  locations: Location[];
  originName: string | null;
  onSubmit: (values: ReturnFormValues) => Promise<void>;
  onCancel: () => void;
}

export function ReturnForm({ locations, originName, onSubmit, onCancel }: ReturnFormProps) {
  const [formError, setFormError] = useState<string | null>(null);
  const {
    register,
    control,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ReturnFormValues>({ resolver: zodResolver(returnSchema), defaultValues: { return_location_id: "", note: "" } });

  const submit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      setFormError(applyApiError(error, setError, ["return_location_id", "note"]));
    }
  });

  const defaultLabel = originName ? `Donde estaba (${originName})` : "Donde estaba";
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <ControlledSelect
        control={control}
        name="return_location_id"
        label="Devolver a"
        error={errors.return_location_id?.message}
        placeholder={defaultLabel}
        clearLabel={defaultLabel}
        options={locations.map((location) => ({ value: String(location.id), label: locationPath(locations, location.id) ?? location.name }))}
      />
      <FormField label="Nota" hint="Opcional." error={errors.note?.message}>
        <Textarea rows={2} {...register("note")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Registrar devolución" pendingLabel="Registrando…" isSubmitting={isSubmitting} />
    </form>
  );
}
