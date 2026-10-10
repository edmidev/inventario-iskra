import { CategoryForm, LocationForm } from "../../../components/organisms/CatalogForms";
import { ConfirmDeleteSheet } from "../../../components/organisms/ConfirmDeleteSheet";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import type { Category, Location } from "../../../lib/api/schemas";
import {
  useCreateCategory,
  useCreateLocation,
  useDeleteCategory,
  useDeleteLocation,
  useUpdateCategory,
  useUpdateLocation,
} from "../api/admin";

interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CategorySheet({ category, open, onOpenChange }: SheetProps & { category?: Category }) {
  const create = useCreateCategory();
  const update = useUpdateCategory(category?.id ?? 0);
  const toast = useToast();
  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={category ? "Editar categoría" : "Nueva categoría"}
      description={category ? category.name : "El prefijo forma parte de los códigos de sus materiales."}
    >
      <CategoryForm
        category={category}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          if (category) await update.mutateAsync(values);
          else await create.mutateAsync(values);
          toast.success(category ? "Categoría actualizada." : `Categoría ${values.name} creada.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function DeleteCategorySheet({ category, open, onOpenChange }: SheetProps & { category: Category }) {
  const remove = useDeleteCategory(category.id);
  const toast = useToast();
  return (
    <ConfirmDeleteSheet
      open={open}
      onOpenChange={onOpenChange}
      title="Eliminar categoría"
      description={`${category.name} (${category.code_prefix})`}
      onConfirm={async () => {
        await remove.mutateAsync();
        toast.success("Categoría eliminada.");
        onOpenChange(false);
      }}
    >
      <p>Solo se puede eliminar si ningún material (ni siquiera los eliminados) la usa.</p>
    </ConfirmDeleteSheet>
  );
}

export function LocationSheet({
  location,
  locations,
  open,
  onOpenChange,
}: SheetProps & { location?: Location; locations: Location[] }) {
  const create = useCreateLocation();
  const update = useUpdateLocation(location?.id ?? 0);
  const toast = useToast();
  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={location ? "Editar ubicación" : "Nueva ubicación"}
      description={location ? location.name : "Bodegas, contenedores o personas que reciben material en préstamo."}
    >
      <LocationForm
        location={location}
        locations={locations}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const parent_id = values.parent_id ? Number(values.parent_id) : null;
          const payload = { name: values.name.trim(), kind: values.kind, parent_id };
          if (location) await update.mutateAsync(payload);
          else await create.mutateAsync(payload);
          toast.success(location ? "Ubicación actualizada." : `Ubicación ${payload.name} creada.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function DeleteLocationSheet({ location, open, onOpenChange }: SheetProps & { location: Location }) {
  const remove = useDeleteLocation(location.id);
  const toast = useToast();
  return (
    <ConfirmDeleteSheet
      open={open}
      onOpenChange={onOpenChange}
      title="Eliminar ubicación"
      description={location.name}
      onConfirm={async () => {
        await remove.mutateAsync();
        toast.success("Ubicación eliminada.");
        onOpenChange(false);
      }}
    >
      <p>Solo se puede eliminar si no tiene sububicaciones, materiales ni historial de préstamos.</p>
    </ConfirmDeleteSheet>
  );
}
