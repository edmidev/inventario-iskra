import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { z } from "zod";
import { Textarea } from "../atoms/Input";
import { ControlledSelect } from "../molecules/ControlledSelect";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import { QuantityStepper } from "../molecules/QuantityStepper";
import { locationPath } from "../../features/catalog/locationPath";
import {
  movementCopy,
  movementFieldNames,
  movementFormSchema,
  resultingStock,
  type MovementFormValues,
  type QuantityMovement,
} from "../../features/stock/movementForm";
import type { Location } from "../../lib/api/schemas";
import { applyApiError } from "../../lib/forms";

export interface StockSubject {
  unit: string | null;
  stock_current: number;
  stock_min: number;
}

interface MovementFormProps {
  mode: QuantityMovement;
  subject: StockSubject;
  /** Should reject with an ApiError on failure; 422 fields are mapped onto the inputs. */
  onSubmit: (values: MovementFormValues) => Promise<void>;
  onCancel: () => void;
}

const QUICK = [1, 5, 10, 25];

export function MovementForm({ mode, subject, onSubmit, onCancel }: MovementFormProps) {
  const copy = movementCopy[mode];
  const unit = subject.unit ?? "";
  const [formError, setFormError] = useState<string | null>(null);
  const {
    register,
    control,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<MovementFormValues>({
    resolver: zodResolver(movementFormSchema(mode)),
    defaultValues: { qty: mode === "adjust" ? String(subject.stock_current) : "1", note: "" },
  });
  const qty = useWatch({ control, name: "qty" });
  const result = resultingStock(mode, subject.stock_current, qty);
  const belowMinimum = result !== null && result >= 0 && subject.stock_min > 0 && result <= subject.stock_min;

  const submit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      setFormError(applyApiError(error, setError, movementFieldNames));
    }
  });

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <p className="rounded-xl bg-sunken px-4 py-3 text-sm">
        Existencias actuales:{" "}
        <strong className="tabular-nums">
          {subject.stock_current} {unit}
        </strong>{" "}
        (mínimo {subject.stock_min})
      </p>

      <FormField label={copy.qtyLabel} required error={errors.qty?.message}>
        {(aria) => (
          <Controller
            control={control}
            name="qty"
            render={({ field }) => (
              <QuantityStepper
                {...aria}
                value={field.value}
                onChange={field.onChange}
                min={mode === "adjust" ? 0 : 1}
                unit={unit}
                quick={mode === "adjust" ? [] : QUICK}
              />
            )}
          />
        )}
      </FormField>

      {result !== null && (
        <div aria-live="polite" className="flex flex-col gap-1 text-sm">
          <p>
            {mode === "adjust" ? "Quedará en" : "Quedarán"}{" "}
            <strong className="tabular-nums">
              {Math.max(result, 0)} {unit}
            </strong>
          </p>
          {result < 0 && <p className="font-medium text-bad">La salida supera las existencias actuales.</p>}
          {belowMinimum && <p className="font-medium text-low">Quedará en o por debajo del stock mínimo.</p>}
        </div>
      )}

      <FormField
        label={mode === "adjust" ? "Motivo del ajuste" : "Nota"}
        required={mode === "adjust"}
        hint={mode === "adjust" ? "Obligatorio: queda en el historial." : "Opcional."}
        error={errors.note?.message}
      >
        <Textarea rows={2} {...register("note")} />
      </FormField>

      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel={copy.submit} pendingLabel={copy.pending} isSubmitting={isSubmitting} />
    </form>
  );
}

const transferSchema = z.object({
  to_location_id: z.string().min(1, "Elige la ubicación de destino."),
  note: z.string().max(500, "Máximo 500 caracteres."),
});
export type TransferFormValues = z.infer<typeof transferSchema>;

interface TransferFormProps {
  locations: Location[];
  currentLocationId: number | null;
  onSubmit: (values: TransferFormValues) => Promise<void>;
  onCancel: () => void;
}

export function TransferForm({ locations, currentLocationId, onSubmit, onCancel }: TransferFormProps) {
  const [formError, setFormError] = useState<string | null>(null);
  const {
    register,
    control,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<TransferFormValues>({ resolver: zodResolver(transferSchema), defaultValues: { to_location_id: "", note: "" } });

  const submit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      setFormError(applyApiError(error, setError, ["to_location_id", "note"]));
    }
  });

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <ControlledSelect
        control={control}
        name="to_location_id"
        label="Ubicación de destino"
        required
        error={errors.to_location_id?.message}
        placeholder="Elige una ubicación"
        options={locations
          .filter((location) => location.id !== currentLocationId)
          .map((location) => ({ value: String(location.id), label: locationPath(locations, location.id) ?? location.name }))}
      />
      <FormField label="Nota" hint="Opcional." error={errors.note?.message}>
        <Textarea rows={2} {...register("note")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Trasladar" pendingLabel="Trasladando…" isSubmitting={isSubmitting} />
    </form>
  );
}
