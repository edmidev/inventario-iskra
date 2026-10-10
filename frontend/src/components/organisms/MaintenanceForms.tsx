import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm, type FieldValues, type Resolver } from "react-hook-form";
import { Input, Textarea } from "../atoms/Input";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import { RadioCards } from "../molecules/RadioCards";
import {
  closeFailureSchema,
  nowInput,
  preventiveSchema,
  reportFailureSchema,
  updateFailureSchema,
  type CloseFailureValues,
  type PreventiveValues,
  type ReportFailureValues,
  type UpdateFailureValues,
} from "../../features/maintenance/maintenanceForm";
import { applyApiError } from "../../lib/forms";
import type { MaintenanceRecord } from "../../lib/api/schemas";

interface BaseProps<T extends FieldValues> {
  /** Should reject with an ApiError on failure; 422 fields are mapped onto the inputs. */
  onSubmit: (values: T) => Promise<void>;
  onCancel: () => void;
}

/** Shared state machine of the maintenance forms: validate, submit, map API errors. */
function useOperationForm<T extends FieldValues>(
  resolver: Resolver<T>,
  defaultValues: T,
  fields: readonly string[],
  onSubmit: (values: T) => Promise<void>,
) {
  const [formError, setFormError] = useState<string | null>(null);
  const form = useForm<T>({ resolver, defaultValues: defaultValues as never });
  const submit = form.handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      setFormError(applyApiError(error, form.setError, fields));
    }
  });
  return { form, submit, formError, setFormError };
}

const failureStatusOptions = [
  { value: "inoperative", label: "Inoperativo" },
  { value: "in_repair", label: "En reparación" },
];

export function ReportFailureForm({ onSubmit, onCancel }: BaseProps<ReportFailureValues>) {
  const { form, submit, formError } = useOperationForm<ReportFailureValues>(
    zodResolver(reportFailureSchema) as Resolver<ReportFailureValues>,
    { description: "", destination: "", item_status: "inoperative", reported_at: nowInput() },
    ["description", "destination", "item_status", "reported_at"],
    onSubmit,
  );
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="¿Qué falló?" required error={errors.description?.message}>
        <Textarea rows={3} {...register("description")} />
      </FormField>
      <RadioCards legend="Estado del material" registration={register("item_status")} options={failureStatusOptions} error={errors.item_status?.message} />
      <FormField label="Destino" hint="Opcional. Por ejemplo: Taller interno o Espera de repuestos." error={errors.destination?.message}>
        <Input autoComplete="off" {...register("destination")} />
      </FormField>
      <FormField label="Fecha de la falla" error={errors.reported_at?.message}>
        <Input type="datetime-local" max={nowInput()} {...register("reported_at")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Reportar falla" pendingLabel="Reportando…" isSubmitting={isSubmitting} danger />
    </form>
  );
}

export function UpdateFailureForm({ record, onSubmit, onCancel }: BaseProps<UpdateFailureValues> & { record: MaintenanceRecord }) {
  const { form, submit, formError } = useOperationForm<UpdateFailureValues>(
    zodResolver(updateFailureSchema) as Resolver<UpdateFailureValues>,
    {
      description: record.description,
      destination: record.destination ?? "",
      item_status: record.item_status === "in_repair" ? "in_repair" : "inoperative",
    },
    ["description", "destination", "item_status"],
    onSubmit,
  );
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <RadioCards legend="Estado del material" registration={register("item_status")} options={failureStatusOptions} error={errors.item_status?.message} />
      <FormField label="Destino" hint="Déjalo vacío para quitarlo." error={errors.destination?.message}>
        <Input autoComplete="off" {...register("destination")} />
      </FormField>
      <FormField label="Descripción de la falla" required error={errors.description?.message}>
        <Textarea rows={3} {...register("description")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Guardar cambios" pendingLabel="Guardando…" isSubmitting={isSubmitting} />
    </form>
  );
}

export function CloseFailureForm({ onSubmit, onCancel }: BaseProps<CloseFailureValues>) {
  const { form, submit, formError } = useOperationForm<CloseFailureValues>(
    zodResolver(closeFailureSchema) as Resolver<CloseFailureValues>,
    { cost: "", resolution_note: "", item_status_after: "functional" },
    ["cost", "resolution_note", "item_status_after", "closed_at"],
    onSubmit,
  );
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <RadioCards
        legend="El material queda"
        registration={register("item_status_after")}
        options={[
          { value: "functional", label: "Funcional" },
          { value: "inoperative", label: "Inoperativo" },
        ]}
        error={errors.item_status_after?.message}
      />
      <FormField label="Costo" hint="Opcional." error={errors.cost?.message}>
        <Input inputMode="decimal" placeholder="0.00" {...register("cost")} />
      </FormField>
      <FormField label="Nota de resolución" hint="Opcional." error={errors.resolution_note?.message}>
        <Textarea rows={3} {...register("resolution_note")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Cerrar falla" pendingLabel="Cerrando…" isSubmitting={isSubmitting} />
    </form>
  );
}

export function PreventiveForm({ onSubmit, onCancel }: BaseProps<PreventiveValues>) {
  const { form, submit, formError } = useOperationForm<PreventiveValues>(
    zodResolver(preventiveSchema) as Resolver<PreventiveValues>,
    { performed_at: nowInput(), description: "", cost: "", next_due_at: "" },
    ["performed_at", "description", "cost", "next_due_at"],
    onSubmit,
  );
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="¿Qué se hizo?" required error={errors.description?.message}>
        <Textarea rows={3} {...register("description")} />
      </FormField>
      <FormField label="Fecha realizada" error={errors.performed_at?.message}>
        <Input type="datetime-local" max={nowInput()} {...register("performed_at")} />
      </FormField>
      <FormField label="Costo" hint="Opcional." error={errors.cost?.message}>
        <Input inputMode="decimal" placeholder="0.00" {...register("cost")} />
      </FormField>
      <FormField
        label="Próximo mantenimiento"
        hint="Opcional. Si lo dejas vacío se calcula con el intervalo del material."
        error={errors.next_due_at?.message}
      >
        <Input type="date" {...register("next_due_at")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Registrar mantenimiento" pendingLabel="Registrando…" isSubmitting={isSubmitting} />
    </form>
  );
}
