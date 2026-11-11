import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { PagedList } from "../components/molecules/PagedList";
import { SearchInput } from "../components/molecules/SearchInput";
import { useSession } from "../features/auth/session";
import { useProjects, type Project } from "../features/projects/api/projects";
import { SaveProjectSheet, DeleteProjectSheet } from "../features/projects/components/ProjectSheets";
import { describeError } from "../lib/api/errors";
export function ProjectsPage() {
  const {
    state
  } = useSession();
  const canWrite = state.user?.permissions.includes("projects:write") ?? false;
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const query = useProjects(q);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Project | null>(null);
  const [deleting, setDeleting] = useState<Project | null>(null);
  const rows = query.data?.pages.flatMap(p => p.data) ?? [];
  return <div className="flex flex-col gap-4"><Link to="/mas" className="min-h-11 font-semibold text-accent">Más</Link>
<div className="flex items-center justify-between gap-3"><div><h1 className="text-2xl font-bold">Proyectos</h1>
<p className="text-sm text-ink-soft">Plantillas de fabricación, materiales y herramientas por unidad.</p></div>
{canWrite && <Button variant="primary" onClick={() => setCreating(true)}>Nuevo proyecto</Button>}</div>
<SearchInput label="Buscar proyectos" placeholder="Nombre o descripción" value={q} onChange={value => {
      const next = new URLSearchParams(params);
      if (value) next.set("q", value);else next.delete("q");
      setParams(next, {
        replace: true
      });
    }} />
<PagedList label="Proyectos" status={query.status} errorMessage={query.error ? describeError(query.error) : undefined} isEmpty={rows.length === 0} emptyTitle="Sin proyectos" emptyDescription="Registra una plantilla para preparar futuros trabajos." hasMore={query.hasNextPage} isFetchingMore={query.isFetchingNextPage} onLoadMore={() => void query.fetchNextPage()} onRetry={() => void query.refetch()}>{rows.map(project => <li key={project.id} className="flex flex-col gap-3 rounded-card border border-line bg-surface p-4"><Link to={`/proyectos/${project.id}`} className="font-semibold text-accent">{project.name}</Link>
<p className="text-sm text-ink-soft">{project.description}</p>
<p className="text-sm">{project.estimated_days} días · {project.consumables.length} consumibles · {project.tools.length} herramientas</p>
{canWrite && <div className="flex gap-2"><Button aria-label={`Editar ${project.name}`} onClick={() => setEditing(project)}>Editar</Button>
<Button variant="danger" aria-label={`Eliminar ${project.name}`} onClick={() => setDeleting(project)}>Eliminar</Button></div>}</li>)}</PagedList>
{canWrite && <SaveProjectSheet open={creating} onOpenChange={setCreating} />} {editing && <SaveProjectSheet project={editing} open onOpenChange={open => !open && setEditing(null)} />} {deleting && <DeleteProjectSheet project={deleting} open onOpenChange={open => !open && setDeleting(null)} />}</div>;
}
