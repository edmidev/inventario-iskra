import { useState } from "react";
import { Button } from "../atoms/Button";
import { Select } from "../atoms/Select";
import { FormField } from "../molecules/FormField";
import { Sheet } from "../molecules/Sheet";
import { locationPath } from "../../features/catalog/locationPath";
import { defaultFilters, type ItemFilters, type SortField } from "../../features/items/filters";
import { availabilityLabels, kindLabels, statusLabels } from "../../lib/format";
import type { Category, Location } from "../../lib/api/schemas";

interface ItemFiltersSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  filters: ItemFilters;
  categories: Category[];
  locations: Location[];
  onApply: (filters: ItemFilters) => void;
}

const sortOptions: { value: string; label: string; sort: SortField; order: "asc" | "desc" }[] = [
  { value: "code:asc", label: "Código (A-Z)", sort: "code", order: "asc" },
  { value: "code:desc", label: "Código (Z-A)", sort: "code", order: "desc" },
  { value: "description:asc", label: "Descripción (A-Z)", sort: "description", order: "asc" },
  { value: "updated_at:desc", label: "Modificados recientemente", sort: "updated_at", order: "desc" },
];

const toOptions = (labels: Record<string, string>) => Object.entries(labels).map(([value, label]) => ({ value, label }));

export function ItemFiltersSheet({ open, onOpenChange, filters, categories, locations, onApply }: ItemFiltersSheetProps) {
  // Edits are staged locally and committed with "Aplicar" so the list does not flicker while choosing.
  const [draft, setDraft] = useState(filters);
  const patch = (changes: Partial<ItemFilters>) => setDraft((current) => ({ ...current, ...changes }));
  const toId = (value: string) => (value === "" ? null : Number(value));

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (next) setDraft(filters);
        onOpenChange(next);
      }}
      title="Filtros"
      description="Combina filtros para acotar la lista."
    >
      <div className="flex flex-col gap-4">
        <FormField label="Categoría">
          <Select
            value={draft.categoryId === null ? "" : String(draft.categoryId)}
            onValueChange={(v) => patch({ categoryId: toId(v) })}
            placeholder="Todas"
            clearLabel="Todas"
            options={categories.map((c) => ({ value: String(c.id), label: c.name }))}
          />
        </FormField>
        <FormField label="Ubicación">
          <Select
            value={draft.locationId === null ? "" : String(draft.locationId)}
            onValueChange={(v) => patch({ locationId: toId(v) })}
            placeholder="Todas"
            clearLabel="Todas"
            options={locations.map((l) => ({ value: String(l.id), label: locationPath(locations, l.id) ?? l.name }))}
          />
        </FormField>
        <FormField label="Estado">
          <Select
            value={draft.status ?? ""}
            onValueChange={(v) => patch({ status: (v || null) as ItemFilters["status"] })}
            placeholder="Todos"
            clearLabel="Todos"
            options={toOptions(statusLabels)}
          />
        </FormField>
        <FormField label="Tipo">
          <Select
            value={draft.kind ?? ""}
            onValueChange={(v) => patch({ kind: (v || null) as ItemFilters["kind"] })}
            placeholder="Todos"
            clearLabel="Todos"
            options={toOptions(kindLabels)}
          />
        </FormField>
        <FormField label="Disponibilidad">
          <Select
            value={draft.availability ?? ""}
            onValueChange={(v) => patch({ availability: (v || null) as ItemFilters["availability"] })}
            placeholder="Todas"
            clearLabel="Todas"
            options={toOptions(availabilityLabels)}
          />
        </FormField>
        <label className="flex min-h-11 items-center gap-3 text-sm font-semibold">
          <input
            type="checkbox"
            className="size-5 accent-[var(--accent)]"
            checked={draft.lowStock}
            onChange={(event) => patch({ lowStock: event.target.checked })}
          />
          Solo stock bajo
        </label>
        <FormField label="Ordenar por">
          <Select
            value={`${draft.sort}:${draft.order}`}
            onValueChange={(v) => {
              const option = sortOptions.find((o) => o.value === v);
              if (option) patch({ sort: option.sort, order: option.order });
            }}
            placeholder="Ordenar"
            options={sortOptions.map(({ value, label }) => ({ value, label }))}
          />
        </FormField>
        <div className="flex gap-3 pt-1">
          <Button
            className="flex-1"
            onClick={() => setDraft({ ...defaultFilters, q: draft.q })}
          >
            Limpiar
          </Button>
          <Button
            variant="primary"
            className="flex-1"
            onClick={() => {
              onApply(draft);
              onOpenChange(false);
            }}
          >
            Aplicar
          </Button>
        </div>
      </div>
    </Sheet>
  );
}
