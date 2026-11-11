import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import { ConfirmDeleteSheet } from "../../../components/organisms/ConfirmDeleteSheet";
import { ProjectForm } from "./ProjectForm";
import { useSaveProject, useDeleteProject, type Project } from "../api/projects";
export function SaveProjectSheet({
  project,
  open,
  onOpenChange
}: {
  project?: Project;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const save = useSaveProject(project?.id);
  const toast = useToast();
  return <Sheet open={open} onOpenChange={onOpenChange} title={project ? "Editar proyecto" : "Nuevo proyecto"}><ProjectForm project={project} onCancel={() => onOpenChange(false)} onSubmit={async payload => {
      await save.mutateAsync(payload);
      toast.success(project ? "Proyecto actualizado." : "Proyecto creado.");
      onOpenChange(false);
    }} /></Sheet>;
}
export function DeleteProjectSheet({
  project,
  open,
  onOpenChange
}: {
  project: Project;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const remove = useDeleteProject(project.id);
  const toast = useToast();
  return <ConfirmDeleteSheet open={open} onOpenChange={onOpenChange} title="Eliminar proyecto" description={project.name} onConfirm={async () => {
    await remove.mutateAsync();
    toast.success("Proyecto eliminado.");
    onOpenChange(false);
  }}>Se elimina la plantilla, no el historial de ejecuciones ya registradas.</ConfirmDeleteSheet>;
}
