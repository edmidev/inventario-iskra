import { ConfirmDeleteSheet } from "../../../components/organisms/ConfirmDeleteSheet";
import { useToast } from "../../../components/molecules/Toast";
import type { Customer } from "../schemas";
import { useDeleteCustomer, useUpdateCustomer } from "../api/customers";
interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}
export function DeleteCustomerSheet({
  customer,
  open,
  onOpenChange
}: SheetProps & {
  customer: Customer;
}) {
  const remove = useDeleteCustomer(customer.id);
  const toast = useToast();
  return <ConfirmDeleteSheet open={open} onOpenChange={onOpenChange} title="Eliminar cliente" description={customer.name} onConfirm={async () => {
    await remove.mutateAsync();
    toast.success(`Cliente ${customer.name} eliminado.`);
    onOpenChange(false);
  }}>
      El historial de este cliente seguirá disponible. Si solo quieres dejar de usarlo, mejor desactívalo.
    </ConfirmDeleteSheet>;
}

/** Flips the active flag without opening the full form. */
export function useToggleCustomerActive(customer: Customer) {
  const update = useUpdateCustomer(customer.id);
  const toast = useToast();
  return async () => {
    try {
      await update.mutateAsync({
        active: !customer.active
      });
      toast.success(customer.active ? `Cliente ${customer.name} desactivado.` : `Cliente ${customer.name} activado.`);
    } catch {
      toast.error("No se pudo cambiar el estado del cliente.");
    }
  };
}
