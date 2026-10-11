import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { Textarea } from "../atoms/Input";
import { ControlledSelect } from "../molecules/ControlledSelect";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import { QuantityStepper } from "../molecules/QuantityStepper";
import { locationChoices, locationDisplayName, stockAt } from "../../features/stock/stocks";
import {
  stockTransferSchema,
  transferFieldNames,
  type TransferFormValues,
} from "../../features/stock/movementForm";
import { ApiError } from "../../lib/api/errors";
import type { ItemStock, Location } from "../../lib/api/schemas";
import { applyApiError } from "../../lib/forms";

interface StockTransferFormProps {
  unit: string | null;
  stocks: ItemStock[];
  locations: Location[];
  /** Should reject with an ApiError on failure; 422 fields are mapped onto the inputs. */
  onSubmit: (values: TransferFormValues) => Promise<void>;
  onCancel: () => void;
}

/** Moves a quantity of a consumable from one location to another; the total does not change. */
export function StockTransferForm({ unit, stocks, locations, onSubmit, onCancel }: StockTransferFormProps) {
  const [formError, setFormError] = useState<string | null>(null);
  const suffix = unit ? ` ${unit}` : "";
  const onlyOrigin = stocks.length === 1 ? String(stocks[0].location_id) : "";
  const {
    register,
    control,
    handleSubmit,
    setError,
    setValue,
    getValues,
    formState: { errors, isSubmitting },
  } = useForm<TransferFormValues>({
    resolver: zodResolver(stockTransferSchema(stocks)),
    defaultValues: { from_location_id: onlyOrigin, to_location_id: "", qty: "1", note: "" },
  });
  const from = useWatch({ control, name: "from_location_id" });
  const to = useWatch({ control, name: "to_location_id" });
  const qty = useWatch({ control, name: "qty" });
  const available = stockAt(stocks, from);
  const amount = /^\d+$/.test(qty.trim()) ? Number(qty) : null;
  const fromName = from ? locationDisplayName(locations, stocks, from) : "";
  const toName = to ? locationDisplayName(locations, stocks, to) : "";
  const ready = amount !== null && amount >= 1 && amount <= available && Boolean(from) && Boolean(to) && from !== to;

  const submit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      if (error instanceof ApiError && error.code === "location_required") {
        setError("from_location_id", { type: "server", message: "Elige la ubicación de origen." });
      } else if (error instanceof ApiError && error.code === "insufficient_stock") {
        setFormError(`No hay existencias suficientes en ${fromName || "el origen"}: hay ${available}${suffix}.`);
      } else {
        setFormError(applyApiError(error, setError, transferFieldNames));
      }
    }
  });

  if (stocks.length === 0) {
    return (
      <div className="flex flex-col gap-4">
        <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
          Este material no tiene existencias en ninguna ubicación: no hay nada que trasladar.
        </p>
        <FormFooter onCancel={onCancel} submitLabel="Trasladar" pendingLabel="Trasladando…" isSubmitting={false} disabled />
      </div>
    );
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <ControlledSelect
        control={control}
        name="from_location_id"
        label="Ubicación de origen"
        required
        error={errors.from_location_id?.message}
        placeholder="Elige el origen"
        options={locationChoices({ locations, stocks, unit, onlyStocked: true })}
        onValueChange={(next) => {
          const max = stockAt(stocks, next);
          if (Number(getValues("qty")) > max) setValue("qty", String(max));
          if (getValues("to_location_id") === next) setValue("to_location_id", "");
        }}
      />

      <ControlledSelect
        control={control}
        name="to_location_id"
        label="Ubicación de destino"
        required
        error={errors.to_location_id?.message}
        placeholder="Elige el destino"
        options={locationChoices({ locations, stocks, unit, exclude: from })}
      />

      <FormField
        label="Cantidad a trasladar"
        required
        error={errors.qty?.message}
        hint={from ? `Disponible en ${fromName}: ${available}${suffix}.` : "Elige primero el origen."}
      >
        {(aria) => (
          <Controller
            control={control}
            name="qty"
            render={({ field }) => (
              <QuantityStepper {...aria} value={field.value} onChange={field.onChange} min={1} max={from ? available : undefined} unit={unit} />
            )}
          />
        )}
      </FormField>

      {ready && (
        <div aria-live="polite" className="flex flex-col gap-1 rounded-xl bg-sunken px-4 py-3 text-sm">
          <p className="font-semibold">
            Mover {amount}
            {suffix} de {fromName} a {toName}
          </p>
          <p className="text-ink-soft">
            Quedarán {available - amount}
            {suffix} en {fromName} y {stockAt(stocks, to) + amount}
            {suffix} en {toName}.
          </p>
        </div>
      )}

      <FormField label="Nota" hint="Opcional." error={errors.note?.message}>
        <Textarea rows={2} {...register("note")} />
      </FormField>

      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel="Trasladar" pendingLabel="Trasladando…" isSubmitting={isSubmitting} />
    </form>
  );
}
