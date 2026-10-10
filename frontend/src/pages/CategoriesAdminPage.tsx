import { useState } from "react";
import { Link } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { IconButton } from "../components/atoms/IconButton";
import { ListSkeleton } from "../components/molecules/ListSkeleton";
import { EmptyState, ErrorState } from "../components/molecules/StateMessage";
import { useCategories } from "../features/catalog/api/catalog";
import { CategorySheet, DeleteCategorySheet } from "../features/catalog/components/CatalogSheets";
import { describeError } from "../lib/api/errors";
import type { Category } from "../lib/api/schemas";

export function CategoriesAdminPage() {
  const query = useCategories();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Category | null>(null);
  const [deleting, setDeleting] = useState<Category | null>(null);

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/mas" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Más
        </Link>
        <div className="flex items-center justify-between gap-3">
          <h1 className="text-2xl font-bold">Categorías</h1>
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Icon name="plus" className="size-4" /> Nueva
          </Button>
        </div>
      </div>

      {query.isPending && <ListSkeleton label="Cargando categorías" />}
      {query.isError && <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />}
      {query.isSuccess && query.data.length === 0 && (
        <EmptyState title="Aún no hay categorías" description="Crea la primera para poder registrar materiales." />
      )}
      {query.isSuccess && query.data.length > 0 && (
        <ul aria-label="Categorías" className="flex flex-col gap-2">
          {query.data.map((category) => (
            <li key={category.id} className="flex items-center justify-between gap-2 rounded-card border border-line bg-surface py-2 pr-2 pl-4">
              <div className="min-w-0">
                <p className="font-semibold break-words">{category.name}</p>
                <Badge tone="accent">{category.code_prefix}</Badge>
              </div>
              <div className="flex shrink-0">
                <IconButton icon="edit" label={`Editar ${category.name}`} onClick={() => setEditing(category)} />
                <IconButton icon="trash" label={`Eliminar ${category.name}`} className="text-bad" onClick={() => setDeleting(category)} />
              </div>
            </li>
          ))}
        </ul>
      )}

      <CategorySheet open={creating} onOpenChange={setCreating} />
      {editing && <CategorySheet category={editing} open onOpenChange={(open) => !open && setEditing(null)} />}
      {deleting && <DeleteCategorySheet category={deleting} open onOpenChange={(open) => !open && setDeleting(null)} />}
    </div>
  );
}
