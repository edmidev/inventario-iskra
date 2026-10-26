import { SupplierForm } from "../../../components/organisms/SupplierForms";
import { ConfirmDeleteSheet } from "../../../components/organisms/ConfirmDeleteSheet";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import type { Supplier } from "../../../lib/api/schemas";
import { useCreateSupplier, useDeleteSupplier, useUpdateSupplier, type SupplierPayload } from "../api/suppliers";

interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function CreateSupplierSheet({ open, onOpenChange }: SheetProps) {
  const create = useCreateSupplier();
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Nuevo proveedor">
      <SupplierForm
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const payload: SupplierPayload = { name: values.name, active: values.active };
          if (values.contact_name) payload.contact_name = values.contact_name;
          if (values.email) payload.email = values.email;
          if (values.phone) payload.phone = values.phone;
          if (values.notes) payload.notes = values.notes;
          const saved = await create.mutateAsync(payload);
          toast.success(`Proveedor ${saved.name} creado.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function EditSupplierSheet({ supplier, open, onOpenChange }: SheetProps & { supplier: Supplier }) {
  const update = useUpdateSupplier(supplier.id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Editar proveedor" description={supplier.name}>
      <SupplierForm
        supplier={supplier}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const payload: SupplierPayload = {};
          for (const key of ["name", "contact_name", "email", "phone", "notes"] as const) {
            if (values[key] !== supplier[key]) payload[key] = values[key];
          }
          if (values.active !== supplier.active) payload.active = values.active;
          if (Object.keys(payload).length > 0) await update.mutateAsync(payload);
          toast.success("Proveedor actualizado.");
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function DeleteSupplierSheet({ supplier, open, onOpenChange }: SheetProps & { supplier: Supplier }) {
  const remove = useDeleteSupplier(supplier.id);
  const toast = useToast();
  return (
    <ConfirmDeleteSheet
      open={open}
      onOpenChange={onOpenChange}
      title="Eliminar proveedor"
      description={supplier.name}
      onConfirm={async () => {
        await remove.mutateAsync();
        toast.success(`Proveedor ${supplier.name} eliminado.`);
        onOpenChange(false);
      }}
    >
      Las órdenes de compra de este proveedor seguirán visibles. Si solo quieres dejar de usarlo, mejor desactívalo.
    </ConfirmDeleteSheet>
  );
}

/** Flips the active flag without opening the full form. */
export function useToggleSupplierActive(supplier: Supplier) {
  const update = useUpdateSupplier(supplier.id);
  const toast = useToast();
  return async () => {
    try {
      await update.mutateAsync({ active: !supplier.active });
      toast.success(supplier.active ? `Proveedor ${supplier.name} desactivado.` : `Proveedor ${supplier.name} activado.`);
    } catch {
      toast.error("No se pudo cambiar el estado del proveedor.");
    }
  };
}
