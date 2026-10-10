import { AssignForm, ReturnForm } from "../../../components/organisms/LoanForms";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import { endOfDayIso } from "../../../lib/dates";
import { useCatalogLookup } from "../../catalog/api/catalog";
import { useAssign, useReturn } from "../api/custody";

interface SubjectProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface AssignSheetProps extends SubjectProps {
  item: { id: string; code: string; description: string };
}

export function AssignSheet({ item, open, onOpenChange }: AssignSheetProps) {
  const assign = useAssign(item.id);
  const toast = useToast();
  const catalog = useCatalogLookup();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Prestar material" description={`${item.code} · ${item.description}`}>
      <AssignForm
        locations={catalog.locations}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const note = values.note.trim();
          const saved = await assign.mutateAsync({
            assignee_location_id: Number(values.assignee_location_id),
            ...(values.due_at ? { due_at: endOfDayIso(values.due_at) } : {}),
            ...(note ? { note } : {}),
          });
          toast.success(`${item.code} prestado a ${saved.assignee_name}.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

interface ReturnSheetProps extends SubjectProps {
  item: { id: string; code: string; description: string };
  /** Where it was before the loan, shown as the default destination. */
  originName?: string | null;
}

export function ReturnSheet({ item, originName = null, open, onOpenChange }: ReturnSheetProps) {
  const giveBack = useReturn(item.id);
  const toast = useToast();
  const catalog = useCatalogLookup();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Registrar devolución" description={`${item.code} · ${item.description}`}>
      <ReturnForm
        locations={catalog.locations}
        originName={originName}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const note = values.note.trim();
          await giveBack.mutateAsync({
            ...(values.return_location_id ? { return_location_id: Number(values.return_location_id) } : {}),
            ...(note ? { note } : {}),
          });
          toast.success(`${item.code} devuelto.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}
