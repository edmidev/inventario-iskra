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
import { defaultLocation, locationChoices, locationDisplayName, stockAt } from "../../features/stock/stocks";
import {
  movementCopy,
  movementFieldNames,
  movementFormSchema,
  resultingStock,
  type MovementFormValues,
  type QuantityMovement,
} from "../../features/stock/movementForm";
import { ApiError } from "../../lib/api/errors";
import type { ItemStock, Location } from "../../lib/api/schemas";
import { applyApiError } from "../../lib/forms";

export interface StockSubject {
  unit: string | null;
  stock_current: number;
  stock_min: number;
  /** Quantity per location (only locations with stock). */
  stocks: ItemStock[];
}

interface MovementFormProps {
  mode: QuantityMovement;
  subject: StockSubject;
  locations: Location[];
  /** Location preselected when the item is not stocked in exactly one place (e.g. the last one used). */
  lastLocation?: string;
  /** Should reject with an ApiError on failure; 422 fields are mapped onto the inputs. */
  onSubmit: (values: MovementFormValues) => Promise<void>;
  onCancel: () => void;
}

const QUICK = [1, 5, 10, 25];

export function MovementForm({ mode, subject, locations, lastLocation, onSubmit, onCancel }: MovementFormProps) {
  const copy = movementCopy[mode];
  const unit = subject.unit ?? "";
  const [formError, setFormError] = useState<string | null>(null);
  const options = locationChoices({ locations, stocks: subject.stocks, unit: subject.unit, onlyStocked: mode === "out" });
  const initialLocation = defaultLocation(subject.stocks, lastLocation);
  const startLocation = options.some((option) => option.value === initialLocation) ? initialLocation : "";
  const {
    register,
    control,
    handleSubmit,
    setError,
    setValue,
    formState: { errors, isSubmitting },
  } = useForm<MovementFormValues>({
    resolver: zodResolver(movementFormSchema(mode)),
    defaultValues: {
      location_id: startLocation,
      qty: mode === "adjust" ? String(stockAt(subject.stocks, startLocation)) : "1",
      note: "",
    },
  });
  const qty = useWatch({ control, name: "qty" });
  const locationId = useWatch({ control, name: "location_id" });
  const locationLabel = locationId ? locationDisplayName(locations, subject.stocks, locationId) : "";
  const here = stockAt(subject.stocks, locationId);
  const result = locationId ? resultingStock(mode, here, qty) : null;
  const totalAfter = result === null ? null : subject.stock_current - here + Math.max(result, 0);
  const tooMuch = result !== null && result < 0;
  const belowMinimum = totalAfter !== null && !tooMuch && subject.stock_min > 0 && totalAfter <= subject.stock_min;
  const nothingToTake = mode === "out" && subject.stocks.length === 0;

  const submit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      if (error instanceof ApiError && error.code === "location_required") {
        setError("location_id", { type: "server", message: "Elige la ubicación." });
      } else if (error instanceof ApiError && error.code === "insufficient_stock" && locationId) {
        setFormError(`No hay existencias suficientes en ${locationLabel}: hay ${here} ${unit}.`.trim());
      } else {
        setFormError(applyApiError(error, setError, movementFieldNames));
      }
    }
  });

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <p className="rounded-xl bg-sunken px-4 py-3 text-sm">
        Existencias totales:{" "}
        <strong className="tabular-nums">
          {subject.stock_current} {unit}
        </strong>{" "}
        (mínimo {subject.stock_min})
      </p>

      {nothingToTake ? (
        <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
          Este material no tiene existencias en ninguna ubicación: no hay nada que sacar.
        </p>
      ) : (
        <ControlledSelect
          control={control}
          name="location_id"
          label={mode === "out" ? "Ubicación de la salida" : "Ubicación"}
          required
          error={errors.location_id?.message}
          placeholder="Elige una ubicación"
          options={options}
          onValueChange={(next) => {
            if (mode === "adjust") setValue("qty", String(stockAt(subject.stocks, next)));
          }}
        />
      )}

      {locationId && (
        <p className="text-sm" aria-live="polite">
          En {locationLabel}:{" "}
          <strong className="tabular-nums">
            {here} {unit}
          </strong>
        </p>
      )}

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
            </strong>{" "}
            en {locationLabel}
            {totalAfter !== null && !tooMuch && (
              <span className="text-ink-soft">
                {" "}
                · total {totalAfter} {unit}
              </span>
            )}
          </p>
          {tooMuch && (
            <p className="font-medium text-bad">
              La salida supera las existencias de {locationLabel} ({here} {unit}).
            </p>
          )}
          {belowMinimum && <p className="font-medium text-low">El total quedará en o por debajo del stock mínimo.</p>}
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
      <FormFooter
        onCancel={onCancel}
        submitLabel={copy.submit}
        pendingLabel={copy.pending}
        isSubmitting={isSubmitting}
        disabled={tooMuch || nothingToTake}
      />
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
