import { useState } from "react";
import { Link, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Select } from "../components/atoms/Select";
import { PagedList } from "../components/molecules/PagedList";
import { SearchInput } from "../components/molecules/SearchInput";
import { useSession } from "../features/auth/session";
import { useCustomers } from "../features/customers/api/customers";
import { DeleteCustomerSheet, useToggleCustomerActive } from "../features/customers/components/CustomerSheets";
import { describeError } from "../lib/api/errors";
import type { Customer } from "../features/customers/schemas";
const activeOptions = [{
  value: "true",
  label: "Activos"
}, {
  value: "false",
  label: "Inactivos"
}];
function CustomerRow({
  customer,
  canOperate,
  onEdit,
  onDelete
}: {
  customer: Customer;
  canOperate: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const toggle = useToggleCustomerActive(customer);
  return <li className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-semibold break-words">{customer.name}</p>
          {customer.identification && <p className="text-sm text-ink-soft">{customer.identification}</p>}
          {customer.contact && <p className="text-sm break-all text-ink-soft">{customer.contact}</p>}
        </div>
        {!customer.active && <Badge tone="warn">Inactivo</Badge>}
      </div>
      {canOperate && <div className="flex flex-wrap gap-2">
          <Button className="flex-1" aria-label={`Editar ${customer.name}`} onClick={onEdit}>
            <Icon name="edit" className="size-4" /> Editar
          </Button>
          <Button className="flex-1" aria-label={`${customer.active ? "Desactivar" : "Activar"} ${customer.name}`} onClick={() => void toggle()}>
            {customer.active ? "Desactivar" : "Activar"}
          </Button>
          <Button className="flex-1" variant="danger" aria-label={`Eliminar ${customer.name}`} onClick={onDelete}>
            Eliminar
          </Button>
        </div>}
    </li>;
}
export function CustomersPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const returnTo = location.pathname + location.search;
  const {
    state
  } = useSession();
  const canOperate = state.user?.permissions.includes("customers:write") ?? false;
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const active = params.get("active") === "true" || params.get("active") === "false" ? params.get("active") as string : "";
  const set = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);else next.delete(key);
    setParams(next, {
      replace: true
    });
  };
  const query = useCustomers({
    q,
    active
  });
  const [deleting, setDeleting] = useState<Customer | null>(null);
  const customers = query.data?.pages.flatMap(page => page.data) ?? [];
  return <div className="flex flex-col gap-4">
      <div>
        <Link to="/mas" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Más
        </Link>
        <div className="flex items-center justify-between gap-3">
          <h1 className="text-2xl font-bold">Clientes</h1>
          {canOperate && <Button variant="primary" onClick={() => navigate("/clientes/nuevo", { state: { returnTo } })}>
              <Icon name="plus" className="size-4" /> Nuevo
            </Button>}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <SearchInput value={q} onChange={value => set("q", value)} label="Buscar clientes" placeholder="Buscar por nombre, contacto o correo" />
        <Select aria-label="Filtrar por estado" value={active} onValueChange={value => set("active", value)} options={activeOptions} placeholder="Todos" clearLabel="Todos" />
      </div>

      <PagedList label="Clientes" status={query.status} errorMessage={query.error ? describeError(query.error) : undefined} isEmpty={customers.length === 0} emptyTitle="Sin clientes" emptyDescription={q || active ? "Prueba con otra búsqueda o quita filtros." : "Registra los clientes para vincularlos a las ejecuciones."} hasMore={query.hasNextPage} isFetchingMore={query.isFetchingNextPage} onLoadMore={() => void query.fetchNextPage()} onRetry={() => void query.refetch()}>
        {customers.map(customer => <CustomerRow key={customer.id} customer={customer} canOperate={canOperate} onEdit={() => navigate(`/clientes/${customer.id}/editar`, { state: { returnTo } })} onDelete={() => setDeleting(customer)} />)}
      </PagedList>

      {deleting && <DeleteCustomerSheet customer={deleting} open onOpenChange={open => !open && setDeleting(null)} />}
    </div>;
}
