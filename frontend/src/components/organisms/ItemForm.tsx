import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useFieldArray, useForm, useWatch } from "react-hook-form";
import { Button } from "../atoms/Button";
import { Icon } from "../atoms/Icon";
import { Input, Textarea } from "../atoms/Input";
import { ControlledSelect } from "../molecules/ControlledSelect";
import { FieldGroup, FormField } from "../molecules/FormField";
import { locationPath } from "../../features/catalog/locationPath";
import {
  formFieldNames,
  itemFormSchema,
  type ItemFormValues,
} from "../../features/items/itemForm";
import { applyApiError } from "../../lib/forms";
import { kindLabels, statusLabels } from "../../lib/format";
import type { Category, Location } from "../../lib/api/schemas";

interface ItemFormProps {
  mode: "create" | "edit";
  initialValues: ItemFormValues;
  categories: Category[];
  locations: Location[];
  /** Only admins may type a custom code, and only when creating. */
  isAdmin: boolean;
  /** Should reject with an ApiError on failure; 422 fields are mapped onto the inputs. */
  onSubmit: (values: ItemFormValues) => Promise<void>;
  onCancel: () => void;
}

export function ItemForm({ mode, initialValues, categories, locations, isAdmin, onSubmit, onCancel }: ItemFormProps) {
  const [formError, setFormError] = useState<string | null>(null);
  const {
    register,
    control,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ItemFormValues>({ resolver: zodResolver(itemFormSchema), defaultValues: initialValues });
  const attributes = useFieldArray({ control, name: "attributes" });
  const kind = useWatch({ control, name: "kind" });

  const submit = handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      setFormError(applyApiError(error, setError, formFieldNames));
    }
  });

  const attributesError = errors.attributes?.message ?? errors.attributes?.root?.message;

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      {mode === "create" ? (
        <FieldGroup legend="Tipo de material">
          <div className="grid grid-cols-2 gap-2">
            {(["asset", "consumable"] as const).map((value) => (
              <label
                key={value}
                className="flex min-h-11 cursor-pointer items-center justify-center rounded-xl border border-line text-sm font-semibold has-[:checked]:border-accent has-[:checked]:bg-accent-soft has-[:checked]:text-accent has-[:focus-visible]:outline-2"
              >
                <input type="radio" value={value} className="sr-only" {...register("kind")} />
                {kindLabels[value]}
              </label>
            ))}
          </div>
        </FieldGroup>
      ) : (
        <p className="text-sm text-ink-soft">
          Tipo: <strong className="text-ink">{kindLabels[initialValues.kind]}</strong> (no se puede cambiar)
        </p>
      )}

      {mode === "create" && isAdmin && (
        <FormField label="Código personalizado" hint="Opcional. Déjalo vacío para generarlo automáticamente." error={errors.code?.message}>
          <Input autoCapitalize="characters" placeholder="ISK-HER-001" {...register("code")} />
        </FormField>
      )}

      <FormField label="Descripción" required error={errors.description?.message}>
        <Input autoComplete="off" {...register("description")} />
      </FormField>

      <ControlledSelect
        control={control}
        name="category_id"
        label="Categoría"
        required
        error={errors.category_id?.message}
        placeholder="Elige una categoría"
        options={categories.map((c) => ({ value: String(c.id), label: c.name }))}
      />

      <ControlledSelect
        control={control}
        name="location_id"
        label="Ubicación"
        error={errors.location_id?.message}
        placeholder="Sin ubicación"
        clearLabel="Sin ubicación"
        options={locations.map((l) => ({ value: String(l.id), label: locationPath(locations, l.id) ?? l.name }))}
      />

      {mode === "edit" && (
        <ControlledSelect
          control={control}
          name="status"
          label="Estado"
          error={errors.status?.message}
          placeholder="Estado"
          options={Object.entries(statusLabels).map(([value, label]) => ({ value, label }))}
        />
      )}

      {kind === "consumable" ? (
        <>
          <FormField label="Unidad" required error={errors.unit?.message}>
            <Input placeholder="pza, caja, litro…" {...register("unit")} />
          </FormField>
          <div className="grid grid-cols-2 gap-3">
            {mode === "create" && (
              <FormField label="Existencias iniciales" error={errors.stock_current?.message}>
                <Input type="number" inputMode="numeric" min={0} {...register("stock_current")} />
              </FormField>
            )}
            <FormField label="Stock mínimo" error={errors.stock_min?.message}>
              <Input type="number" inputMode="numeric" min={0} {...register("stock_min")} />
            </FormField>
          </div>
        </>
      ) : (
        <FormField
          label="Mantenimiento cada (días)"
          hint="Opcional. Se usa para calcular el próximo mantenimiento."
          error={errors.maintenance_interval_days?.message}
        >
          <Input type="number" inputMode="numeric" min={1} max={3650} {...register("maintenance_interval_days")} />
        </FormField>
      )}

      <FieldGroup legend="Atributos (marca, color, serie…)" error={attributesError}>
        {attributes.fields.map((row, index) => (
          <div key={row.id} className="flex items-start gap-2">
            <Input aria-label={`Nombre del atributo ${index + 1}`} placeholder="Marca" {...register(`attributes.${index}.key`)} />
            <Input aria-label={`Valor del atributo ${index + 1}`} placeholder="Makita" {...register(`attributes.${index}.value`)} />
            <Button variant="ghost" aria-label={`Quitar atributo ${index + 1}`} className="px-3" onClick={() => attributes.remove(index)}>
              <Icon name="x" />
            </Button>
          </div>
        ))}
        <Button variant="secondary" className="self-start" onClick={() => attributes.append({ key: "", value: "" })}>
          <Icon name="plus" className="size-4" /> Añadir atributo
        </Button>
      </FieldGroup>

      <FormField label="Notas" error={errors.notes?.message}>
        <Textarea {...register("notes")} />
      </FormField>

      {formError && (
        <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
          {formError}
        </p>
      )}

      <div className="sticky bottom-0 -mx-5 flex gap-3 border-t border-line bg-surface px-5 pt-3">
        <Button variant="secondary" className="flex-1" onClick={onCancel}>
          Cancelar
        </Button>
        <Button type="submit" variant="primary" className="flex-1" disabled={isSubmitting}>
          {isSubmitting ? "Guardando…" : mode === "create" ? "Crear material" : "Guardar cambios"}
        </Button>
      </div>
    </form>
  );
}
