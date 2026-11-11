import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Button } from "../components/atoms/Button";
import { LoadingState, ErrorState } from "../components/molecules/StateMessage";
import { useSession } from "../features/auth/session";
import { useProject } from "../features/projects/api/projects";
import { SaveProjectSheet } from "../features/projects/components/ProjectSheets";
import { describeError } from "../lib/api/errors";
export function ProjectDetailPage() {
  const id = Number(useParams().id);
  const query = useProject(id);
  const {
    state
  } = useSession();
  const [editing, setEditing] = useState(false);
  if (!Number.isSafeInteger(id) || id < 1) return <ErrorState message="Proyecto no encontrado." />;
  if (query.isPending) return <LoadingState label="Cargando proyecto" />;
  if (query.isError) return <ErrorState message={describeError(query.error)} onRetry={() => void query.refetch()} />;
  const p = query.data;
  return <div className="flex flex-col gap-4"><Link to="/proyectos" className="min-h-11 font-semibold text-accent">Proyectos</Link>
<h1 className="text-2xl font-bold">{p.name}</h1>
<p>{p.description}</p>
<p className="text-sm text-ink-soft">Tiempo estimado: {p.estimated_days} días. Puede ajustarse al registrar una ejecución.</p>
<section aria-label="Características"><h2 className="font-semibold">Características</h2>
<ul className="list-inside list-disc">{p.characteristics.map((c, i) => <li key={`${i}-${c}`}>{c}</li>)}</ul></section>
<section aria-label="Consumibles por unidad"><h2 className="font-semibold">Consumibles por unidad</h2>
<ul>{p.consumables.map(m => <li key={m.item_id}><Link to={`/materiales/${m.item_id}`} className="text-accent">{m.item_id}</Link> · {m.quantity}</li>)}</ul></section>
<section aria-label="Herramientas"><h2 className="font-semibold">Herramientas</h2>
<p className="text-sm text-ink-soft">Listado informativo. Los equipos pueden usarse en varios trabajos.</p>
<ul>{p.tools.map(toolId => <li key={toolId}><Link to={`/materiales/${toolId}`} className="text-accent">{toolId}</Link></li>)}</ul></section>
{state.user?.permissions.includes("projects:write") && <Button onClick={() => setEditing(true)}>Editar proyecto</Button>}
<SaveProjectSheet project={p} open={editing} onOpenChange={setEditing} /></div>;
}
