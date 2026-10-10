import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Icon } from "../components/atoms/Icon";
import { useToast } from "../components/molecules/Toast";
import { ErrorState, LoadingState } from "../components/molecules/StateMessage";
import { ItemDetailView } from "../components/organisms/ItemDetailView";
import { usePermissions } from "../features/auth/session";
import { useCatalogLookup } from "../features/catalog/api/catalog";
import { HistoryTab } from "../features/history/components/HistoryTab";
import { MaintenanceTab } from "../features/maintenance/components/MaintenanceTab";
import { useItem } from "../features/items/api/items";
import { DeleteItemSheet } from "../features/items/components/DeleteItemSheet";
import { ItemFormSheet } from "../features/items/components/ItemFormSheet";
import { ApiError, describeError } from "../lib/api/errors";

export function ItemDetailPage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const { canWriteItems } = usePermissions();
  const catalog = useCatalogLookup();
  const query = useItem(id);
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const back = (
    <Link to="/materiales" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
      <Icon name="chevron" className="size-4 rotate-180" /> Materiales
    </Link>
  );

  let content;
  if (query.isPending) {
    content = <LoadingState label="Cargando material" />;
  } else if (query.isError) {
    const notFound = query.error instanceof ApiError && query.error.status === 404;
    content = (
      <ErrorState
        message={notFound ? "Este material no existe o fue eliminado." : describeError(query.error)}
        onRetry={notFound ? undefined : () => void query.refetch()}
      />
    );
  } else {
    const item = query.data;
    content = (
      <>
        <ItemDetailView
          item={item}
          categoryName={catalog.categoryName(item.category_id)}
          locationName={catalog.locationName(item.location_id)}
          canWrite={canWriteItems}
          historyTab={<HistoryTab item={item} />}
          maintenanceTab={<MaintenanceTab item={item} />}
          onEdit={() => setEditing(true)}
          onDelete={() => setDeleting(true)}
          onCopyCode={() => {
            navigator.clipboard
              ?.writeText(item.code)
              .then(() => toast.success(`Código ${item.code} copiado.`))
              .catch(() => toast.error("No se pudo copiar el código."));
          }}
        />
        {canWriteItems && (
          <>
            <ItemFormSheet open={editing} onOpenChange={setEditing} item={item} />
            <DeleteItemSheet
              item={item}
              open={deleting}
              onOpenChange={setDeleting}
              onDeleted={() => {
                toast.success("Material eliminado.");
                navigate("/materiales", { replace: true });
              }}
            />
          </>
        )}
      </>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      {back}
      {content}
    </div>
  );
}
