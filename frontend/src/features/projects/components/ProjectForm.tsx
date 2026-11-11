import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { Controller, useFieldArray, type Resolver } from "react-hook-form";
import { z } from "zod";
import { Button } from "../../../components/atoms/Button";
import { Input, Textarea } from "../../../components/atoms/Input";
import { Select } from "../../../components/atoms/Select";
import { FormField, FieldGroup } from "../../../components/molecules/FormField";
import { FormAlert, FormFooter } from "../../../components/molecules/FormFooter";
import { useApiForm } from "../../../lib/useApiForm";
import { useTemplateItems, type Project, type ProjectPayload } from "../api/projects";
const positiveQuantity = z.string().regex(/^\d+(?:\.\d{1,3})?$/, "Usa hasta tres decimales.").refine(v => Number(v) > 0 && Number(v) <= 1_000_000_000, "Indica una cantidad mayor que cero.");
const formSchema = z.object({
  name: z.string().trim().min(1, "Escribe el nombre.").max(120),
  description: z.string().trim().max(4000),
  characteristics: z.string(),
  estimated_days: z.string().regex(/^\d+$/, "Indica días enteros.").refine(v => Number(v) > 0 && Number(v) <= 36500, "Indica entre 1 y 36500 días."),
  consumables: z.array(z.object({
    item_id: z.string().min(1, "Selecciona un consumible."),
    quantity: positiveQuantity
  })),
  tools: z.array(z.object({
    item_id: z.string().min(1, "Selecciona una herramienta.")
  }))
});
type Values = z.infer<typeof formSchema>;
function ItemPicker({
  kind,
  value,
  onChange,
  label
}: {
  kind: "asset" | "consumable";
  value: string;
  onChange: (id: string) => void;
  label: string;
}) {
  const [q, setQ] = useState("");
  const items = useTemplateItems(kind, q);
  const options = (items.data?.pages.flatMap(p => p.data) ?? []).map(i => ({
    value: i.id,
    label: `${i.code} · ${i.description}`
  }));
  if (value && !options.some(o => o.value === value)) options.unshift({
    value,
    label: value
  });
  return <div className="flex flex-col gap-2"><Input aria-label={`Buscar ${label}`} value={q} onChange={e => setQ(e.target.value)} placeholder="Buscar por código o descripción" />
<Select aria-label={label} value={value} onValueChange={onChange} options={options} placeholder={items.isPending ? "Cargando inventario…" : "Seleccionar del inventario"} />
{items.isError && <p className="text-sm text-bad">No se pudo consultar el inventario. <Button onClick={() => void items.refetch()}>Reintentar</Button></p>}
{items.hasNextPage && <Button onClick={() => void items.fetchNextPage()} disabled={items.isFetchingNextPage}>Cargar más artículos</Button>}</div>;
}

/** A per-unit bill of materials, deliberately distinct from live execution usage. */
export function ProjectForm({
  project,
  onSubmit,
  onCancel
}: {
  project?: Project;
  onSubmit: (payload: ProjectPayload) => Promise<void>;
  onCancel: () => void;
}) {
  const {
    form,
    submit,
    formError
  } = useApiForm<Values>({
    resolver: zodResolver(formSchema) as Resolver<Values>,
    defaultValues: {
      name: project?.name ?? "",
      description: project?.description ?? "",
      characteristics: project?.characteristics.join("\n") ?? "",
      estimated_days: String(project?.estimated_days ?? 1),
      consumables: project?.consumables.map(m => ({
        ...m,
        quantity: String(m.quantity)
      })) ?? [],
      tools: project?.tools.map(item_id => ({
        item_id
      })) ?? []
    },
    fields: ["name", "description", "characteristics", "estimated_days", "consumables.*.item_id", "consumables.*.quantity", "tools.*.item_id"],
    codeFields: {
      project_name_taken: "name"
    },
    onSubmit: async v => onSubmit({
      ...v,
      characteristics: v.characteristics.split("\n").map(s => s.trim()).filter(Boolean),
      estimated_days: Number(v.estimated_days),
      consumables: v.consumables.map(m => ({
        ...m,
        quantity: Number(m.quantity)
      })),
      tools: v.tools.map(t => t.item_id)
    })
  });
  const materials = useFieldArray({
    control: form.control,
    name: "consumables"
  });
  const tools = useFieldArray({
    control: form.control,
    name: "tools"
  });
  const {
    errors,
    isSubmitting
  } = form.formState;
  return <form onSubmit={submit} noValidate className="flex flex-col gap-4">
 <FormField label="Nombre" required error={errors.name?.message}><Input {...form.register("name")} /></FormField>
 <FormField label="Descripción" error={errors.description?.message}><Textarea rows={3} {...form.register("description")} /></FormField>
 <FormField label="Características" hint="Una característica por línea." error={errors.characteristics?.message}><Textarea rows={3} {...form.register("characteristics")} /></FormField>
 <FormField label="Tiempo estimado (días)" required error={errors.estimated_days?.message}><Input inputMode="numeric" {...form.register("estimated_days")} /></FormField>
 <FieldGroup legend="Consumibles por unidad"><p className="text-sm text-ink-soft">Las cantidades se multiplicarán por las unidades a fabricar.</p>
{materials.fields.map((m, i) => <div key={m.id} className="flex flex-col gap-2 rounded-xl border border-line p-3"><Controller control={form.control} name={`consumables.${i}.item_id`} render={({
          field
        }) => <ItemPicker kind="consumable" value={field.value} onChange={field.onChange} label={`Consumible ${i + 1}`} />} />
{errors.consumables?.[i]?.item_id && <p role="alert" className="text-sm text-bad">{errors.consumables[i]?.item_id?.message}</p>}
<FormField label={`Cantidad ${i + 1}`} required error={errors.consumables?.[i]?.quantity?.message}><Input inputMode="decimal" {...form.register(`consumables.${i}.quantity`)} /></FormField>
<Button onClick={() => materials.remove(i)}>Quitar consumible {i + 1}</Button></div>)}
<Button onClick={() => materials.append({
        item_id: "",
        quantity: "1"
      })}>Agregar consumible</Button></FieldGroup>
 <FieldGroup legend="Herramientas necesarias"><p className="text-sm text-ink-soft">Selecciona equipos concretos. No se reservan ni se multiplican por unidad.</p>
{tools.fields.map((t, i) => <div key={t.id} className="flex flex-col gap-2 rounded-xl border border-line p-3"><Controller control={form.control} name={`tools.${i}.item_id`} render={({
          field
        }) => <ItemPicker kind="asset" value={field.value} onChange={field.onChange} label={`Herramienta ${i + 1}`} />} />
{errors.tools?.[i]?.item_id && <p role="alert" className="text-sm text-bad">{errors.tools[i]?.item_id?.message}</p>}
<Button onClick={() => tools.remove(i)}>Quitar herramienta {i + 1}</Button></div>)}
<Button onClick={() => tools.append({
        item_id: ""
      })}>Agregar herramienta</Button></FieldGroup>
 <FormAlert message={formError} />
<FormFooter onCancel={onCancel} submitLabel={project ? "Guardar cambios" : "Crear proyecto"} pendingLabel="Guardando…" isSubmitting={isSubmitting} />
 </form>;
}
