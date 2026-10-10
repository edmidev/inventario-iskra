import { zodResolver } from "@hookform/resolvers/zod";
import { useMemo } from "react";
import type { Resolver } from "react-hook-form";
import { Input } from "../atoms/Input";
import { ControlledSelect } from "../molecules/ControlledSelect";
import { FormAlert, FormFooter } from "../molecules/FormFooter";
import { FormField } from "../molecules/FormField";
import { categorySchema, locationFormSchema, type CategoryFormInput, type CategoryFormValues, type LocationFormValues } from "../../features/catalog/catalogForm";
import { descendantIds, locationKindLabels } from "../../features/catalog/locationTree";
import { locationPath } from "../../features/catalog/locationPath";
import { useApiForm } from "../../lib/useApiForm";
import type { Category, Location } from "../../lib/api/schemas";

interface BaseProps<T> {
  onSubmit: (values: T) => Promise<void>;
  onCancel: () => void;
}

export function CategoryForm({ category, onSubmit, onCancel }: BaseProps<CategoryFormValues> & { category?: Category }) {
  const { form, submit, formError } = useApiForm<CategoryFormInput, CategoryFormValues>({
    resolver: zodResolver(categorySchema) as Resolver<CategoryFormInput, unknown, CategoryFormValues>,
    defaultValues: { name: category?.name ?? "", code_prefix: category?.code_prefix ?? "" },
    fields: ["name", "code_prefix"],
    codeFields: { category_name_taken: "name", category_prefix_taken: "code_prefix" },
    onSubmit,
  });
  const { register, formState } = form;
  const { errors, isSubmitting } = formState;
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="Nombre" required error={errors.name?.message}>
        <Input autoComplete="off" {...register("name")} />
      </FormField>
      <FormField
        label="Prefijo del código"
        required
        hint="De 2 a 8 letras o números; se guarda en mayúsculas. Solo afecta a los códigos que se generen después."
        error={errors.code_prefix?.message}
      >
        <Input autoComplete="off" autoCapitalize="characters" spellCheck={false} maxLength={8} className="font-mono uppercase" {...register("code_prefix")} />
      </FormField>
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel={category ? "Guardar cambios" : "Crear categoría"} pendingLabel="Guardando…" isSubmitting={isSubmitting} />
    </form>
  );
}

const kindOptions = (Object.keys(locationKindLabels) as Location["kind"][]).map((value) => ({ value, label: locationKindLabels[value] }));

export function LocationForm({
  location,
  locations,
  onSubmit,
  onCancel,
}: BaseProps<LocationFormValues> & { location?: Location; locations: Location[] }) {
  const { form, submit, formError } = useApiForm<LocationFormValues>({
    resolver: zodResolver(locationFormSchema) as Resolver<LocationFormValues>,
    defaultValues: {
      name: location?.name ?? "",
      kind: location?.kind ?? "storage",
      parent_id: location?.parent_id === null || location?.parent_id === undefined ? "" : String(location.parent_id),
    },
    fields: ["name", "kind", "parent_id"],
    codeFields: { location_cycle: "parent_id" },
    onSubmit,
  });
  const { register, control, formState } = form;
  const { errors, isSubmitting } = formState;
  const parentOptions = useMemo(() => {
    const blocked = location ? descendantIds(locations, location.id) : new Set<number>();
    return locations
      .filter((l) => !blocked.has(l.id))
      .map((l) => ({ value: String(l.id), label: locationPath(locations, l.id) ?? l.name }))
      .sort((a, b) => a.label.localeCompare(b.label, "es"));
  }, [location, locations]);
  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-4">
      <FormField label="Nombre" required error={errors.name?.message}>
        <Input autoComplete="off" {...register("name")} />
      </FormField>
      <ControlledSelect control={control} name="kind" label="Tipo" placeholder="Elige un tipo" options={kindOptions} required error={errors.kind?.message} />
      <ControlledSelect
        control={control}
        name="parent_id"
        label="Dentro de"
        placeholder="Sin ubicación padre"
        clearLabel="Sin ubicación padre"
        options={parentOptions}
        error={errors.parent_id?.message}
      />
      <FormAlert message={formError} />
      <FormFooter onCancel={onCancel} submitLabel={location ? "Guardar cambios" : "Crear ubicación"} pendingLabel="Guardando…" isSubmitting={isSubmitting} />
    </form>
  );
}
