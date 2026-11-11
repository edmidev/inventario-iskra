import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { Icon } from "../components/atoms/Icon";
import { EmptyState, ErrorState, LoadingState } from "../components/molecules/StateMessage";
import { useToast } from "../components/molecules/Toast";
import { useSession } from "../features/auth/session";
import { useCreateCustomer, useCustomer, useUpdateCustomer, type CustomerPayload } from "../features/customers/api/customers";
import { CustomerForm } from "../features/customers/components/CustomerForm";
import type { CustomerValues } from "../features/customers/schemas";
import { describeError } from "../lib/api/errors";

export function CustomerFormPage() {
  const { id } = useParams();
  const editing = id !== undefined;
  const customerID = Number(id);
  const { state } = useSession();
  const canWrite = state.user?.permissions.includes("customers:write") ?? false;
  const customer = useCustomer(customerID, editing && canWrite);
  const create = useCreateCustomer();
  const update = useUpdateCustomer(customerID);
  const toast = useToast();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { returnTo?: unknown } | null)?.returnTo;
  const returnTo = typeof from === "string" && (from === "/clientes" || from.startsWith("/clientes?")) ? from : "/clientes";
  const back = () => navigate(returnTo);

  async function submit(values: CustomerValues) {
    if (editing && customer.data) {
      const payload: CustomerPayload = {};
      for (const key of ["name", "identification", "contact", "notes"] as const) {
        if (values[key] !== customer.data[key]) payload[key] = values[key];
      }
      if (values.active !== customer.data.active) payload.active = values.active;
      if (Object.keys(payload).length > 0) await update.mutateAsync(payload);
      toast.success("Cliente actualizado.");
    } else if (!editing) {
      const payload: CustomerPayload = { name: values.name, active: values.active };
      if (values.identification) payload.identification = values.identification;
      if (values.contact) payload.contact = values.contact;
      if (values.notes) payload.notes = values.notes;
      const saved = await create.mutateAsync(payload);
      toast.success(`Cliente ${saved.name} creado.`);
    }
    back();
  }

  return <div className="mx-auto flex w-full max-w-xl flex-col gap-4">
    <Link to={returnTo} className="inline-flex min-h-11 items-center gap-1 self-start font-semibold text-accent"><Icon name="chevron" className="size-4 rotate-180" /> Volver a clientes</Link>
    <h1 className="text-2xl font-bold">{editing ? "Editar cliente" : "Nuevo cliente"}</h1>
    {!canWrite ? <EmptyState title="Sin acceso" description="Tu rol no tiene permiso para modificar clientes." /> :
      editing && (!Number.isInteger(customerID) || customerID < 1) ? <ErrorState message="Cliente no válido." /> :
      editing && customer.isLoading ? <LoadingState label="Cargando cliente" /> :
      editing && customer.isError ? <ErrorState message={describeError(customer.error)} onRetry={() => void customer.refetch()} /> :
      (!editing || customer.data) && <CustomerForm customer={customer.data} onSubmit={submit} onCancel={back} />}
  </div>;
}
