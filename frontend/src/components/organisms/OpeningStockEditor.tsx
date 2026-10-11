import { Controller, useFieldArray, useWatch, type Control, type FieldErrors, type UseFormRegister } from "react-hook-form";
import { Button } from "../atoms/Button";
import { Icon } from "../atoms/Icon";
import { Input } from "../atoms/Input";
import { Select } from "../atoms/Select";
import { FieldGroup } from "../molecules/FormField";
import { locationPath } from "../../features/catalog/locationPath";
import type { ItemFormValues } from "../../features/items/itemForm";
import type { Location } from "../../lib/api/schemas";

interface OpeningStockEditorProps {
  control: Control<ItemFormValues>;
  register: UseFormRegister<ItemFormValues>;
  errors: FieldErrors<ItemFormValues>;
  locations: Location[];
}

/** One row per location holding the opening balance of a new consumable; at most one row per location. */
export function OpeningStockEditor({ control, register, errors, locations }: OpeningStockEditorProps) {
  const rows = useFieldArray({ control, name: "stocks" });
  const watched = useWatch({ control, name: "stocks" });
  const unit = useWatch({ control, name: "unit" }).trim();
  const total = watched.reduce((sum, row) => sum + (/^\d+$/.test(row.quantity.trim()) ? Number(row.quantity) : 0), 0);
  const groupError = errors.stocks?.message ?? errors.stocks?.root?.message;
  const taken = (index: number) => new Set(watched.filter((_, i) => i !== index).map((row) => row.location_id).filter(Boolean));

  return (
    <FieldGroup legend="Stock inicial por ubicación" error={groupError}>
      <p className="-mt-1 text-xs text-ink-soft">
        Opcional. Indica cuántas unidades hay en cada ubicación; puedes dejarlo vacío si aún no hay existencias.
      </p>
      {rows.fields.map((row, index) => {
        const rowErrors = errors.stocks?.[index];
        const unavailable = taken(index);
        return (
          <div key={row.id} className="flex flex-col gap-2 rounded-xl border border-line p-3">
            <div className="flex items-start gap-2">
              <div className="min-w-0 flex-1">
                <Controller
                  control={control}
                  name={`stocks.${index}.location_id`}
                  render={({ field }) => (
                    <Select
                      aria-label={`Ubicación del stock ${index + 1}`}
                      value={field.value}
                      onValueChange={field.onChange}
                      invalid={Boolean(rowErrors?.location_id)}
                      placeholder="Elige una ubicación"
                      options={locations
                        .filter((location) => !unavailable.has(String(location.id)))
                        .map((location) => ({ value: String(location.id), label: locationPath(locations, location.id) ?? location.name }))}
                    />
                  )}
                />
              </div>
              <Button variant="ghost" aria-label={`Quitar ubicación ${index + 1}`} className="min-h-11 min-w-11 px-3" onClick={() => rows.remove(index)}>
                <Icon name="x" />
              </Button>
            </div>
            <div className="relative">
              <Input
                type="number"
                inputMode="numeric"
                min={1}
                aria-label={`Cantidad del stock ${index + 1}`}
                invalid={Boolean(rowErrors?.quantity)}
                className={unit ? "pr-16" : undefined}
                {...register(`stocks.${index}.quantity`)}
              />
              {unit && <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-sm text-ink-soft">{unit}</span>}
            </div>
            {[rowErrors?.location_id?.message, rowErrors?.quantity?.message].filter(Boolean).map((message) => (
              <p key={message} role="alert" className="text-sm font-medium text-bad">
                {message}
              </p>
            ))}
          </div>
        );
      })}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Button
          variant="secondary"
          className="self-start"
          disabled={rows.fields.length >= locations.length}
          onClick={() => rows.append({ location_id: "", quantity: "1" })}
        >
          <Icon name="plus" className="size-4" /> Añadir ubicación
        </Button>
        {rows.fields.length > 0 && (
          <p aria-live="polite" className="text-sm">
            Total: <strong className="tabular-nums">{total}{unit ? ` ${unit}` : ""}</strong>
          </p>
        )}
      </div>
    </FieldGroup>
  );
}
