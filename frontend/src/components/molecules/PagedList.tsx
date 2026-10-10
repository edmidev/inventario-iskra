import type { ReactNode } from "react";
import { Button } from "../atoms/Button";
import { EmptyState, ErrorState } from "./StateMessage";
import { ListSkeleton } from "./ListSkeleton";

interface PagedListProps {
  label: string;
  status: "pending" | "error" | "success";
  errorMessage?: string;
  isEmpty: boolean;
  emptyTitle: string;
  emptyDescription?: string;
  hasMore: boolean;
  isFetchingMore: boolean;
  onLoadMore: () => void;
  onRetry: () => void;
  children: ReactNode;
}

/** Loading / error / empty / list + "Cargar más" for the operational pages. */
export function PagedList(props: PagedListProps) {
  if (props.status === "pending") return <ListSkeleton label={`Cargando ${props.label}`} />;
  if (props.status === "error") return <ErrorState message={props.errorMessage ?? "No se pudo cargar la lista."} onRetry={props.onRetry} />;
  if (props.isEmpty) return <EmptyState title={props.emptyTitle} description={props.emptyDescription} />;
  return (
    <div className="flex flex-col gap-3">
      <ul aria-label={props.label} className="flex flex-col gap-2">
        {props.children}
      </ul>
      {props.hasMore && (
        <Button block onClick={props.onLoadMore} disabled={props.isFetchingMore}>
          {props.isFetchingMore ? "Cargando…" : "Cargar más"}
        </Button>
      )}
    </div>
  );
}
