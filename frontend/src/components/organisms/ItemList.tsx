import { Button } from "../atoms/Button";
import { EmptyState, ErrorState, LoadingState } from "../molecules/StateMessage";
import { ItemCard, ItemListHeader } from "../molecules/ItemCard";
import type { Item } from "../../lib/api/schemas";

interface ItemListProps {
  items: Item[];
  total: number;
  status: "pending" | "error" | "success";
  errorMessage?: string;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  hasFilters: boolean;
  categoryName: (id: number) => string | null;
  locationName: (id: number | null) => string | null;
  onLoadMore: () => void;
  onRetry: () => void;
  onClearFilters: () => void;
}

export function ItemList(props: ItemListProps) {
  if (props.status === "pending") return <LoadingState label="Cargando materiales" />;
  if (props.status === "error") {
    return <ErrorState message={props.errorMessage ?? "No se pudo cargar la lista."} onRetry={props.onRetry} />;
  }
  if (props.items.length === 0) {
    return (
      <EmptyState
        title={props.hasFilters ? "Sin resultados" : "Aún no hay materiales"}
        description={props.hasFilters ? "Prueba con otra búsqueda o quita algunos filtros." : undefined}
        action={
          props.hasFilters ? (
            <Button onClick={props.onClearFilters}>Quitar filtros</Button>
          ) : undefined
        }
      />
    );
  }
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-ink-soft" aria-live="polite">
        {props.items.length} de {props.total} materiales
      </p>
      <div className="md:overflow-hidden md:rounded-card md:border md:border-line md:bg-surface">
        <ItemListHeader />
        <ul className="flex flex-col gap-2 md:gap-0">
          {props.items.map((item) => (
            <ItemCard
              key={item.id}
              item={item}
              categoryName={props.categoryName(item.category_id)}
              locationName={props.locationName(item.location_id)}
            />
          ))}
        </ul>
      </div>
      {props.hasNextPage && (
        <Button onClick={props.onLoadMore} disabled={props.isFetchingNextPage} block>
          {props.isFetchingNextPage ? "Cargando…" : "Cargar más"}
        </Button>
      )}
    </div>
  );
}
