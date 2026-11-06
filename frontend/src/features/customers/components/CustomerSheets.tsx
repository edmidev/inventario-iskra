import { CustomerForm } from "./CustomerForm";
import { ConfirmDeleteSheet } from "../../../components/organisms/ConfirmDeleteSheet";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import type { Customer } from "../schemas";
import { useCreateCustomer, useDeleteCustomer, useUpdateCustomer, type CustomerPayload } from "../api/customers";
interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}
export function CreateCustomerSheet({
  open,
  onOpenChange,
  onCreated
}: SheetProps & {
  onCreated?: (customer: Customer) => void;
}) {
  const create = useCreateCustomer();
  const toast = useToast();
  return <Sheet open={open} onOpenChange={onOpenChange} title="Nuevo cliente">
      <CustomerForm onCancel={() => onOpenChange(false)} onSubmit={async values => {
      const payload: CustomerPayload = {
        name: values.name,
        active: values.active
      };
      if (values.identification) payload.identification = values.identification;
      if (values.contact) payload.contact = values.contact;
      if (values.notes) payload.notes = values.notes;
      const saved = await create.mutateAsync(payload);
      toast.success(`Cliente ${saved.name} creado.`);
      onCreated?.(saved);
      onOpenChange(false);
    }} />
    </Sheet>;
}
export function EditCustomerSheet({
  customer,
  open,
  onOpenChange
}: SheetProps & {
  customer: Customer;
}) {
  const update = useUpdateCustomer(customer.id);
  const toast = useToast();
  return <Sheet open={open} onOpenChange={onOpenChange} title="Editar cliente" description={customer.name}>
      <CustomerForm customer={customer} onCancel={() => onOpenChange(false)} onSubmit={async values => {
      const payload: CustomerPayload = {};
      for (const key of ["name", "identification", "contact", "notes"] as const) {
        if (values[key] !== customer[key]) payload[key] = values[key];
      }
      if (values.active !== customer.active) payload.active = values.active;
      if (Object.keys(payload).length > 0) await update.mutateAsync(payload);
      toast.success("Cliente actualizado.");
      onOpenChange(false);
    }} />
    </Sheet>;
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
