import { useCallback, useMemo, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { FilterChip } from "../components/molecules/FilterChip";
import { SearchInput } from "../components/molecules/SearchInput";
import { ItemFiltersSheet } from "../components/organisms/ItemFiltersSheet";
import { ItemList } from "../components/organisms/ItemList";
import { Can } from "../features/auth/RoleGate";
import { useCatalogLookup } from "../features/catalog/api/catalog";
import { useItems } from "../features/items/api/items";
import { ItemFormSheet } from "../features/items/components/ItemFormSheet";
import {
  activeFilterCount,
  defaultFilters,
  parseFilters,
  toSearchParams,
  type ItemFilters,
} from "../features/items/filters";
import { describeError } from "../lib/api/errors";
import { availabilityLabels, kindLabels, statusLabels } from "../lib/format";

export function ItemsPage() {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const filters = useMemo(() => parseFilters(params), [params]);
  const catalog = useCatalogLookup();
  const query = useItems(filters);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);

  const setFilters = useCallback(
    (next: ItemFilters) => setParams(toSearchParams(next), { replace: true }),
    [setParams],
  );
  const setSearch = useCallback((q: string) => setFilters({ ...filters, q }), [filters, setFilters]);

  const items = query.data?.pages.flatMap((page) => page.data) ?? [];
  const total = query.data?.pages.at(-1)?.total ?? 0;
  const count = activeFilterCount(filters);
  const hasFilters = count > 0 || filters.q.trim() !== "";

  const chips: { label: string; clear: Partial<ItemFilters> }[] = [];
  if (filters.categoryId !== null)
    chips.push({ label: catalog.categoryName(filters.categoryId) ?? "Categoría", clear: { categoryId: null } });
  if (filters.locationId !== null)
    chips.push({ label: catalog.locationName(filters.locationId) ?? "Ubicación", clear: { locationId: null } });
  if (filters.status) chips.push({ label: statusLabels[filters.status], clear: { status: null } });
  if (filters.kind) chips.push({ label: kindLabels[filters.kind], clear: { kind: null } });
  if (filters.availability) chips.push({ label: availabilityLabels[filters.availability], clear: { availability: null } });
  if (filters.lowStock) chips.push({ label: "Stock bajo", clear: { lowStock: false } });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">Materiales</h1>
        <Can check={(p) => p.canWriteItems}>
          <Button variant="primary" onClick={() => setCreateOpen(true)}>
            <Icon name="plus" className="size-4" /> Nuevo
          </Button>
        </Can>
      </div>

      <div className="sticky top-0 z-20 -mx-4 flex gap-2 bg-canvas/95 px-4 py-2 backdrop-blur">
        <div className="flex-1">
          <SearchInput
            value={filters.q}
            onChange={setSearch}
            label="Buscar materiales"
            placeholder="Buscar por código o descripción"
          />
        </div>
        <Button onClick={() => setFiltersOpen(true)} aria-label={count ? `Filtros (${count} activos)` : "Filtros"}>
          <Icon name="filter" className="size-4" />
          <span className="hidden sm:inline">Filtros</span>
          {count > 0 && <span className="rounded-full bg-accent px-1.5 text-xs text-accent-ink">{count}</span>}
        </Button>
      </div>

      {chips.length > 0 && (
        <ul aria-label="Filtros activos" className="flex flex-wrap gap-2">
          {chips.map((chip) => (
            <li key={chip.label}>
              <FilterChip label={chip.label} onRemove={() => setFilters({ ...filters, ...chip.clear })} />
            </li>
          ))}
        </ul>
      )}

      <ItemList
        items={items}
        total={total}
        status={query.status}
        errorMessage={query.error ? describeError(query.error) : undefined}
        hasNextPage={query.hasNextPage}
        isFetchingNextPage={query.isFetchingNextPage}
        hasFilters={hasFilters}
        categoryName={catalog.categoryName}
        locationName={catalog.locationName}
        onLoadMore={() => void query.fetchNextPage()}
        onRetry={() => void query.refetch()}
        onClearFilters={() => setFilters(defaultFilters)}
      />

      <ItemFiltersSheet
        open={filtersOpen}
        onOpenChange={setFiltersOpen}
        filters={filters}
        categories={catalog.categories}
        locations={catalog.locations}
        onApply={setFilters}
      />
      <Can check={(p) => p.canWriteItems}>
        <ItemFormSheet open={createOpen} onOpenChange={setCreateOpen} onSaved={(item) => navigate(`/materiales/${item.id}`)} />
      </Can>
    </div>
  );
}
