import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { IconButton } from "../components/atoms/IconButton";
import { ListSkeleton } from "../components/molecules/ListSkeleton";
import { EmptyState, ErrorState } from "../components/molecules/StateMessage";
import { useLocations } from "../features/catalog/api/catalog";
import { DeleteLocationSheet, LocationSheet } from "../features/catalog/components/CatalogSheets";
import { buildLocationRows, locationKindLabels } from "../features/catalog/locationTree";
import { describeError } from "../lib/api/errors";
import type { Location } from "../lib/api/schemas";

export function LocationsAdminPage() {
  const query = useLocations();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Location | null>(null);
  const [deleting, setDeleting] = useState<Location | null>(null);
  const rows = useMemo(() => buildLocationRows(query.data ?? []), [query.data]);

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/mas" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Más
        </Link>
        <div className="flex items-center justify-between gap-3">
          <h1 className="text-2xl font-bold">Ubicaciones</h1>
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Icon name="plus" className="size-4" /> Nueva
          </Button>
        </div>
      </div>

      {query.isPending && <ListSkeleton label="Cargando ubicaciones" />}
      {query.isError && <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />}
      {query.isSuccess && rows.length === 0 && (
        <EmptyState title="Aún no hay ubicaciones" description="Crea una bodega o una persona para empezar a ubicar material." />
      )}
      {rows.length > 0 && (
        <ul aria-label="Ubicaciones" className="flex flex-col gap-2">
          {rows.map(({ location, depth }) => (
            <li
              key={location.id}
              style={{ marginLeft: `${Math.min(depth, 4) * 1.25}rem` }}
              className="flex items-center justify-between gap-2 rounded-card border border-line bg-surface py-2 pr-2 pl-4"
            >
              <div className="min-w-0">
                <p className="font-semibold break-words">{location.name}</p>
                <Badge tone={location.kind === "person" ? "warn" : "neutral"}>{locationKindLabels[location.kind]}</Badge>
              </div>
              <div className="flex shrink-0">
                <IconButton icon="edit" label={`Editar ${location.name}`} onClick={() => setEditing(location)} />
                <IconButton icon="trash" label={`Eliminar ${location.name}`} className="text-bad" onClick={() => setDeleting(location)} />
              </div>
            </li>
          ))}
        </ul>
      )}

      <LocationSheet locations={query.data ?? []} open={creating} onOpenChange={setCreating} />
      {editing && <LocationSheet location={editing} locations={query.data ?? []} open onOpenChange={(open) => !open && setEditing(null)} />}
      {deleting && <DeleteLocationSheet location={deleting} open onOpenChange={(open) => !open && setDeleting(null)} />}
    </div>
  );
}
