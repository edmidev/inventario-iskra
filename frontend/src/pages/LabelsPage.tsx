import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Select } from "../components/atoms/Select";
import { LabelCard } from "../components/molecules/LabelCard";
import { SearchInput } from "../components/molecules/SearchInput";
import { EmptyState, ErrorState, LoadingState } from "../components/molecules/StateMessage";
import { PrintArea } from "../components/organisms/PrintArea";
import { useCatalogLookup } from "../features/catalog/api/catalog";
import { useItems } from "../features/items/api/items";
import { defaultFilters } from "../features/items/filters";
import { describeError } from "../lib/api/errors";
import type { Item } from "../lib/api/schemas";

/** Pick several items and print a sheet of ~50x30 mm QR labels. */
export function LabelsPage() {
  const catalog = useCatalogLookup();
  const [q, setQ] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [selected, setSelected] = useState<Map<string, Item>>(new Map());
  const filters = useMemo(() => ({ ...defaultFilters, q, categoryId: categoryId ? Number(categoryId) : null }), [q, categoryId]);
  const query = useItems(filters);
  const items = query.data?.pages.flatMap((page) => page.data) ?? [];

  const toggle = (item: Item) =>
    setSelected((current) => {
      const next = new Map(current);
      if (next.has(item.id)) next.delete(item.id);
      else next.set(item.id, item);
      return next;
    });
  const selectVisible = () =>
    setSelected((current) => {
      const next = new Map(current);
      for (const item of items) next.set(item.id, item);
      return next;
    });
  const chosen = [...selected.values()];

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/materiales" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Materiales
        </Link>
        <h1 className="text-2xl font-bold">Etiquetas QR</h1>
        <p className="text-sm text-ink-soft">Elige los materiales y, al imprimir, se genera una hoja de etiquetas de 50 × 30 mm.</p>
      </div>

      <div className="sticky top-0 z-20 -mx-4 flex flex-col gap-2 bg-canvas/95 px-4 py-2 backdrop-blur">
        <SearchInput value={q} onChange={setQ} label="Buscar materiales" placeholder="Buscar por código o descripción" />
        <Select
          aria-label="Categoría"
          value={categoryId}
          onValueChange={setCategoryId}
          options={catalog.categories.map((c) => ({ value: String(c.id), label: c.name }))}
          placeholder="Todas las categorías"
          clearLabel="Todas las categorías"
        />
      </div>

      {query.isPending && <LoadingState label="Cargando materiales" />}
      {query.isError && <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />}
      {query.isSuccess && items.length === 0 && <EmptyState title="Sin resultados" description="Prueba con otra búsqueda." />}
      {items.length > 0 && (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm text-ink-soft" aria-live="polite">
              {chosen.length} seleccionados
            </p>
            <div className="flex gap-2">
              <Button onClick={selectVisible}>Seleccionar visibles</Button>
              <Button onClick={() => setSelected(new Map())} disabled={chosen.length === 0}>
                Quitar
              </Button>
            </div>
          </div>
          <ul aria-label="Materiales" className="flex flex-col gap-2">
            {items.map((item) => (
              <li key={item.id}>
                <label className="flex min-h-14 cursor-pointer items-center gap-3 rounded-card border border-line bg-surface px-4 py-2">
                  <input
                    type="checkbox"
                    className="size-5 shrink-0 accent-(--accent)"
                    aria-label={`Seleccionar ${item.code}`}
                    checked={selected.has(item.id)}
                    onChange={() => toggle(item)}
                  />
                  <span className="min-w-0">
                    <span className="block font-mono text-sm font-semibold text-accent">{item.code}</span>
                    <span className="block truncate text-sm">{item.description}</span>
                  </span>
                </label>
              </li>
            ))}
          </ul>
          {query.hasNextPage && (
            <Button block onClick={() => void query.fetchNextPage()} disabled={query.isFetchingNextPage}>
              {query.isFetchingNextPage ? "Cargando…" : "Cargar más"}
            </Button>
          )}
        </>
      )}

      <div className="sticky bottom-20 z-20 md:bottom-4">
        <Button variant="primary" block disabled={chosen.length === 0} onClick={() => window.print()}>
          <Icon name="print" className="size-4" /> Imprimir {chosen.length > 0 ? `(${chosen.length})` : ""}
        </Button>
      </div>

      <PrintArea>
        <div className="label-grid">
          {chosen.map((item) => (
            <LabelCard key={item.id} code={item.code} description={item.description} />
          ))}
        </div>
      </PrintArea>
    </div>
  );
}
