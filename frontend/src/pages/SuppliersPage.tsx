import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { Icon } from "../components/atoms/Icon";
import { Select } from "../components/atoms/Select";
import { PagedList } from "../components/molecules/PagedList";
import { SearchInput } from "../components/molecules/SearchInput";
import { usePermissions } from "../features/auth/session";
import { useSuppliers } from "../features/suppliers/api/suppliers";
import {
  CreateSupplierSheet,
  DeleteSupplierSheet,
  EditSupplierSheet,
  useToggleSupplierActive,
} from "../features/suppliers/components/SupplierSheets";
import { describeError } from "../lib/api/errors";
import type { Supplier } from "../lib/api/schemas";

const activeOptions = [
  { value: "true", label: "Activos" },
  { value: "false", label: "Inactivos" },
];

function SupplierRow({ supplier, canOperate, onEdit, onDelete }: { supplier: Supplier; canOperate: boolean; onEdit: () => void; onDelete: () => void }) {
  const toggle = useToggleSupplierActive(supplier);
  return (
    <li className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-semibold break-words">{supplier.name}</p>
          {supplier.contact_name && <p className="text-sm text-ink-soft">{supplier.contact_name}</p>}
          {supplier.email && <p className="text-sm break-all text-ink-soft">{supplier.email}</p>}
          {supplier.phone && <p className="text-sm text-ink-soft">{supplier.phone}</p>}
        </div>
        {!supplier.active && <Badge tone="warn">Inactivo</Badge>}
      </div>
      {canOperate && (
        <div className="flex flex-wrap gap-2">
          <Button className="flex-1" aria-label={`Editar ${supplier.name}`} onClick={onEdit}>
            <Icon name="edit" className="size-4" /> Editar
          </Button>
          <Button className="flex-1" aria-label={`${supplier.active ? "Desactivar" : "Activar"} ${supplier.name}`} onClick={() => void toggle()}>
            {supplier.active ? "Desactivar" : "Activar"}
          </Button>
          <Button className="flex-1" variant="danger" aria-label={`Eliminar ${supplier.name}`} onClick={onDelete}>
            Eliminar
          </Button>
        </div>
      )}
    </li>
  );
}

export function SuppliersPage() {
  const { canOperate } = usePermissions();
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const active = params.get("active") === "true" || params.get("active") === "false" ? (params.get("active") as string) : "";
  const set = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  };
  const query = useSuppliers({ q, active });
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Supplier | null>(null);
  const [deleting, setDeleting] = useState<Supplier | null>(null);
  const suppliers = query.data?.pages.flatMap((page) => page.data) ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link to="/mas" className="inline-flex min-h-11 items-center gap-1 text-sm font-semibold text-accent">
          <Icon name="chevron" className="size-4 rotate-180" /> Más
        </Link>
        <div className="flex items-center justify-between gap-3">
          <h1 className="text-2xl font-bold">Proveedores</h1>
          {canOperate && (
            <Button variant="primary" onClick={() => setCreating(true)}>
              <Icon name="plus" className="size-4" /> Nuevo
            </Button>
          )}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <SearchInput value={q} onChange={(value) => set("q", value)} label="Buscar proveedores" placeholder="Buscar por nombre, contacto o correo" />
        <Select aria-label="Filtrar por estado" value={active} onValueChange={(value) => set("active", value)} options={activeOptions} placeholder="Todos" clearLabel="Todos" />
      </div>

      <PagedList
        label="Proveedores"
        status={query.status}
        errorMessage={query.error ? describeError(query.error) : undefined}
        isEmpty={suppliers.length === 0}
        emptyTitle="Sin proveedores"
        emptyDescription={q || active ? "Prueba con otra búsqueda o quita filtros." : "Crea el primero para poder hacer órdenes de compra."}
        hasMore={query.hasNextPage}
        isFetchingMore={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
        onRetry={() => void query.refetch()}
      >
        {suppliers.map((supplier) => (
          <SupplierRow key={supplier.id} supplier={supplier} canOperate={canOperate} onEdit={() => setEditing(supplier)} onDelete={() => setDeleting(supplier)} />
        ))}
      </PagedList>

      {canOperate && <CreateSupplierSheet open={creating} onOpenChange={setCreating} />}
      {editing && <EditSupplierSheet supplier={editing} open onOpenChange={(open) => !open && setEditing(null)} />}
      {deleting && <DeleteSupplierSheet supplier={deleting} open onOpenChange={(open) => !open && setDeleting(null)} />}
    </div>
  );
}
