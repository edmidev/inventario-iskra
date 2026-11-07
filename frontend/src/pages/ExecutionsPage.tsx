import { useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Badge } from "../components/atoms/Badge";
import { Button } from "../components/atoms/Button";
import { PagedList } from "../components/molecules/PagedList";
import { SearchInput } from "../components/molecules/SearchInput";
import { Sheet } from "../components/molecules/Sheet";
import { useToast } from "../components/molecules/Toast";
import { useSession } from "../features/auth/session";
import { useCustomers } from "../features/customers/api/customers";
import { useAssignableUsers, useCreateExecution, useExecutions, useMoveExecutionMaterial, useTransitionExecution, type Execution, type MaterialAllocation } from "../features/executions/api/executions";
import { useLocations } from "../features/catalog/api/catalog";
import { useProjects, type Project } from "../features/projects/api/projects";
import { describeError } from "../lib/api/errors";

const statusLabels: Record<Execution["status"], string> = {
  registered: "Registrada",
  in_progress: "En ejecución",
  paused: "Pausada",
  completed: "Terminada",
  cancelled: "Cancelada",
};
const statusTones: Record<Execution["status"], "neutral" | "accent" | "ok" | "bad" | "warn"> = {
  registered: "neutral",
  in_progress: "ok",
  paused: "warn",
  completed: "accent",
  cancelled: "bad",
};

function ExecutionActions({ execution }: { execution: Execution }) {
  const transition = useTransitionExecution(execution.id);
  const toast = useToast();
  const action = execution.status === "registered" ? "start" : execution.status === "paused" ? "resume" : execution.status === "in_progress" ? "pause" : null;
  if (!action) return null;
  const label = action === "start" ? "Iniciar" : action === "pause" ? "Pausar" : "Reanudar";
  return <Button disabled={transition.isPending} onClick={async () => {
    try {
      await transition.mutateAsync(action);
      toast.success(action === "start" ? "Ejecución iniciada; el inventario puede completarse durante el trabajo." : `Ejecución ${action === "pause" ? "pausada" : "reanudad"}a.`);
    } catch (error) {
      toast.error(describeError(error));
    }
  }}>{label}</Button>;
}

function MaterialMovementForm({ executionID, material }: { executionID: number; material: MaterialAllocation }) {
  const [action, setAction] = useState<"deliver" | "return">("deliver");
  const [quantity, setQuantity] = useState("");
  const [locationID, setLocationID] = useState("");
  const locations = useLocations();
  const move = useMoveExecutionMaterial(executionID, material.item_id);
  const toast = useToast();

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      await move.mutateAsync({ action, request_id: crypto.randomUUID(), quantity: Number(quantity), location_id: Number(locationID) });
      toast.success(action === "deliver" ? "Material entregado al proyecto." : "Material devuelto al inventario.");
      setQuantity("");
    } catch (error) {
      toast.error(describeError(error));
    }
  }

  return <form onSubmit={submit} className="mt-3 grid gap-2 rounded-xl border border-line p-3 sm:grid-cols-[1fr_1fr_1fr_auto] sm:items-end">
    <label className="flex flex-col gap-1 text-xs font-medium">Movimiento
      <select value={action} onChange={(event) => setAction(event.target.value as "deliver" | "return")} className="min-h-10 rounded-lg border border-line bg-surface px-2 text-sm">
        <option value="deliver">Entregar reservado</option>
        <option value="return">Devolver sobrante</option>
      </select>
    </label>
    <label className="flex flex-col gap-1 text-xs font-medium">Cantidad
      <input required type="number" min="0.001" step="0.001" value={quantity} onChange={(event) => setQuantity(event.target.value)} className="min-h-10 rounded-lg border border-line bg-surface px-2 text-sm" />
    </label>
    <label className="flex flex-col gap-1 text-xs font-medium">Ubicación física
      <select required value={locationID} onChange={(event) => setLocationID(event.target.value)} className="min-h-10 rounded-lg border border-line bg-surface px-2 text-sm">
        <option value="">Seleccioná una ubicación</option>
        {(locations.data ?? []).map((location) => <option key={location.id} value={location.id}>{location.name}</option>)}
      </select>
    </label>
    <Button type="submit" disabled={move.isPending || !locationID || !quantity}>{move.isPending ? "Guardando…" : action === "deliver" ? "Entregar" : "Devolver"}</Button>
    {locations.isError && <p className="text-sm text-bad sm:col-span-4">No se pudieron cargar las ubicaciones físicas.</p>}
    {!locations.isError && !locations.isLoading && locations.data?.length === 0 && <p className="text-sm text-ink-soft sm:col-span-4">Crea una ubicación antes de mover materiales.</p>}
  </form>;
}

function ExecutionCard({ execution, canWrite }: { execution: Execution; canWrite: boolean }) {
  return <li className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4">
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div><h2 className="font-semibold">{execution.project_name}</h2><p className="text-sm text-ink-soft">Ejecución #{execution.id}</p></div>
      <Badge tone={statusTones[execution.status]}>{statusLabels[execution.status]}</Badge>
    </div>
    <p className="text-sm">{execution.units} {execution.units === 1 ? "unidad" : "unidades"} · {execution.estimated_days} días · {execution.customer_ids.length} {execution.customer_ids.length === 1 ? "cliente" : "clientes"}</p>
    <p className="text-sm text-ink-soft">Responsable principal asignado · {execution.worker_ids.length} trabajadores</p>
    <details className="rounded-xl bg-sunken p-3 text-sm">
      <summary className="cursor-pointer font-semibold">Ver snapshot del proyecto</summary>
      <p className="mt-2">{execution.description || "Sin descripción"}</p>
      {execution.characteristics.length > 0 && <ul className="mt-2 list-inside list-disc">{execution.characteristics.map((value, index) => <li key={`${index}-${value}`}>{value}</li>)}</ul>}
      <p className="mt-2">{execution.consumables.length} consumibles · {execution.tools.length} herramientas requeridas</p>
    </details>
    {execution.materials.length > 0 && <section className="rounded-xl bg-sunken p-3">
      <h3 className="font-semibold">Insumos de la ejecución</h3>
      <ul className="mt-2 flex flex-col gap-3">
        {execution.materials.map((material) => <li key={material.item_id} className="rounded-xl border border-line bg-surface p-3 text-sm">
          <p className="font-medium">Artículo {material.item_id}</p>
          <p className="mt-1 text-ink-soft">Requerido: {material.required_quantity} · Reservado: {material.reserved_quantity} · Entregado: {material.delivered_quantity} · Faltante: {material.deficit_quantity}</p>
          {material.returned_quantity > 0 && <p className="text-ink-soft">Devuelto: {material.returned_quantity}</p>}
          {canWrite && (execution.status === "registered" || execution.status === "in_progress" || execution.status === "paused") && <MaterialMovementForm executionID={execution.id} material={material} />}
        </li>)}
      </ul>
    </section>}
    {canWrite && <div className="flex gap-2"><ExecutionActions execution={execution} /></div>}
  </li>;
}

function CreateExecutionSheet({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [projectSearch, setProjectSearch] = useState("");
  const [customerSearch, setCustomerSearch] = useState("");
  const [selectedCustomerIDs, setSelectedCustomerIDs] = useState<number[]>([]);
  const [selectedProject, setSelectedProject] = useState<Project | null>(null);
  const [estimatedDays, setEstimatedDays] = useState("");
  const create = useCreateExecution();
  const toast = useToast();
  const projects = useProjects(projectSearch);
  const customers = useCustomers({ q: customerSearch, active: "true" });
  const users = useAssignableUsers();
  const projectRows = projects.data?.pages.flatMap((page) => page.data) ?? [];
  const customerRows = customers.data?.pages.flatMap((page) => page.data) ?? [];
  const userRows = users.data ?? [];

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    if (selectedCustomerIDs.length === 0) {
      toast.error("Seleccioná al menos un cliente.");
      return;
    }
    try {
      await create.mutateAsync({
        project_id: Number(form.get("project_id")),
        units: Number(form.get("units")),
        ...(form.get("estimated_days") ? { estimated_days: Number(form.get("estimated_days")) } : {}),
        customer_ids: selectedCustomerIDs,
        principal_user_id: String(form.get("principal_user_id") ?? ""),
        worker_ids: form.getAll("worker_ids").map(String),
      });
      toast.success("Ejecución registrada con un snapshot independiente del proyecto.");
      onOpenChange(false);
    } catch (error) {
      toast.error(describeError(error));
    }
  }

  return <Sheet open={open} onOpenChange={onOpenChange} title="Registrar ejecución" description="El trabajo conserva una copia del proyecto; el tiempo se puede ajustar para esta ejecución.">
    <form className="flex flex-col gap-4" onSubmit={submit}>
      <label className="flex flex-col gap-1 text-sm font-medium">Buscar proyecto
        <input value={projectSearch} onChange={(event) => setProjectSearch(event.target.value)} className="min-h-11 rounded-xl border border-line bg-sunken px-3" placeholder="Nombre del proyecto" />
      </label>
      <label className="flex flex-col gap-1 text-sm font-medium">Proyecto
        <select required name="project_id" value={selectedProject?.id ?? ""} onChange={(event) => {
          const project = projectRows.find((candidate) => candidate.id === Number(event.target.value)) ?? null;
          setSelectedProject(project);
          setEstimatedDays(project ? String(project.estimated_days) : "");
        }} className="min-h-11 rounded-xl border border-line bg-sunken px-3">
          <option value="">Seleccioná una plantilla</option>
          {projectRows.map((project) => <option key={project.id} value={project.id}>{project.name} · {project.estimated_days} días</option>)}
        </select>
      </label>
      {projects.hasNextPage && <Button onClick={() => void projects.fetchNextPage()} disabled={projects.isFetchingNextPage}>Cargar más proyectos</Button>}
      <div className="grid grid-cols-2 gap-3">
        <label className="flex flex-col gap-1 text-sm font-medium">Unidades
          <input required name="units" type="number" min="1" step="1" defaultValue="1" className="min-h-11 rounded-xl border border-line bg-sunken px-3" />
        </label>
        <label className="flex flex-col gap-1 text-sm font-medium">Días estimados
          <input name="estimated_days" type="number" min="1" max="36500" step="1" value={estimatedDays} onChange={(event) => setEstimatedDays(event.target.value)} className="min-h-11 rounded-xl border border-line bg-sunken px-3" />
        </label>
      </div>
      <label className="flex flex-col gap-1 text-sm font-medium">Buscar clientes
        <input value={customerSearch} onChange={(event) => setCustomerSearch(event.target.value)} className="min-h-11 rounded-xl border border-line bg-sunken px-3" placeholder="Nombre del cliente" />
      </label>
      <fieldset className="flex flex-col gap-2 rounded-xl border border-line p-3">
        <legend className="px-1 text-sm font-semibold">Clientes (elegí uno o varios)</legend>
        {customerRows.map((customer) => <label key={customer.id} className="flex min-h-10 items-center gap-2"><input type="checkbox" checked={selectedCustomerIDs.includes(customer.id)} onChange={(event) => setSelectedCustomerIDs((current) => event.target.checked ? [...current, customer.id] : current.filter((id) => id !== customer.id))} />{customer.name}</label>)}
        {customerRows.length === 0 && <p className="text-sm text-ink-soft">No hay clientes activos para asignar.</p>}
        {customers.hasNextPage && <Button onClick={() => void customers.fetchNextPage()} disabled={customers.isFetchingNextPage}>Cargar más clientes</Button>}
      </fieldset>
      <label className="flex flex-col gap-1 text-sm font-medium">Responsable principal
        <select required name="principal_user_id" defaultValue="" className="min-h-11 rounded-xl border border-line bg-sunken px-3">
          <option value="">Seleccioná una persona</option>
          {userRows.map((user) => <option key={user.id} value={user.id}>{user.name}</option>)}
        </select>
      </label>
      <fieldset className="flex flex-col gap-2 rounded-xl border border-line p-3">
        <legend className="px-1 text-sm font-semibold">Trabajadores asignados (opcional)</legend>
        {userRows.map((user) => <label key={user.id} className="flex min-h-10 items-center gap-2"><input type="checkbox" name="worker_ids" value={user.id} />{user.name}</label>)}
        {users.isError && <p className="text-sm text-bad">No se pudieron cargar los usuarios activos.</p>}
      </fieldset>
      <p className="text-sm text-ink-soft">Se puede iniciar el trabajo aunque falte inventario; las herramientas requeridas no se reservan.</p>
      <Button type="submit" variant="primary" block disabled={create.isPending || !selectedProject}>{create.isPending ? "Registrando…" : "Registrar ejecución"}</Button>
    </form>
  </Sheet>;
}

export function ExecutionsPage() {
  const { state } = useSession();
  const canWrite = state.user?.permissions.includes("executions:write") ?? false;
  const canReadProjects = state.user?.permissions.includes("projects:read") ?? false;
  const canReadCustomers = state.user?.permissions.includes("customers:read") ?? false;
  const canCreate = canWrite && canReadProjects && canReadCustomers;
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const executions = useExecutions(q);
  const [creating, setCreating] = useState(false);
  const rows = executions.data?.pages.flatMap((page) => page.data) ?? [];
  return <div className="flex flex-col gap-4">
    <Link to="/mas" className="min-h-11 font-semibold text-accent">Más</Link>
    <div className="flex items-center justify-between gap-3"><div><h1 className="text-2xl font-bold">Ejecuciones</h1><p className="text-sm text-ink-soft">Trabajos registrados, responsables y estado del taller.</p></div>{canCreate && <Button variant="primary" onClick={() => setCreating(true)}>Nueva ejecución</Button>}</div>
    {canWrite && !canCreate && <p role="note" className="rounded-card border border-line bg-sunken p-3 text-sm text-ink-soft">Para registrar ejecuciones, el administrador debe asignarte también lectura de proyectos y clientes.</p>}
    <SearchInput label="Buscar ejecuciones" placeholder="Nombre del proyecto" value={q} onChange={(value) => {
      const next = new URLSearchParams(params);
      if (value) next.set("q", value); else next.delete("q");
      setParams(next, { replace: true });
    }} />
    <PagedList label="Ejecuciones" status={executions.status} errorMessage={executions.error ? describeError(executions.error) : undefined} isEmpty={rows.length === 0} emptyTitle="Sin ejecuciones" emptyDescription="Registra una ejecución a partir de una plantilla de proyecto." hasMore={executions.hasNextPage} isFetchingMore={executions.isFetchingNextPage} onLoadMore={() => void executions.fetchNextPage()} onRetry={() => void executions.refetch()}>
      {rows.map((execution) => <ExecutionCard key={execution.id} execution={execution} canWrite={canWrite} />)}
    </PagedList>
    {canCreate && creating && <CreateExecutionSheet open onOpenChange={setCreating} />}
  </div>;
}
