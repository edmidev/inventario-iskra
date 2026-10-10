import { useMemo } from "react";
import { ItemForm } from "../../../components/organisms/ItemForm";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import type { Item } from "../../../lib/api/schemas";
import { useCatalogLookup } from "../../catalog/api/catalog";
import { usePermissions } from "../../auth/session";
import { useCreateItem, useUpdateItem } from "../api/items";
import { emptyItemForm, itemToFormValues, toCreatePayload, toUpdatePayload } from "../itemForm";

interface ItemFormSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Item being edited; omit to create. */
  item?: Item;
  onSaved?: (item: Item) => void;
}

export function ItemFormSheet({ open, onOpenChange, item, onSaved }: ItemFormSheetProps) {
  const { isAdmin } = usePermissions();
  const { categories, locations } = useCatalogLookup();
  const toast = useToast();
  const create = useCreateItem();
  const update = useUpdateItem(item?.id ?? "");
  const initialValues = useMemo(() => (item ? itemToFormValues(item) : emptyItemForm), [item]);

  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={item ? "Editar material" : "Nuevo material"}
      description={item ? item.code : "El código se genera automáticamente."}
    >
      <ItemForm
        key={item ? `${item.id}-${item.updated_at}` : "new"}
        mode={item ? "edit" : "create"}
        initialValues={initialValues}
        categories={categories}
        locations={locations}
        isAdmin={isAdmin}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const saved = item
            ? await update.mutateAsync(toUpdatePayload(values, initialValues))
            : await create.mutateAsync(toCreatePayload(values, isAdmin));
          toast.success(item ? "Material actualizado." : `Material ${saved.code} creado.`);
          onOpenChange(false);
          onSaved?.(saved);
        }}
      />
    </Sheet>
  );
}
