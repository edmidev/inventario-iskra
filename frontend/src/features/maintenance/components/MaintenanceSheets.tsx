import { CloseFailureForm, PreventiveForm, ReportFailureForm, UpdateFailureForm } from "../../../components/organisms/MaintenanceForms";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import { dateTimeInputToIso, startOfDayIso } from "../../../lib/dates";
import type { MaintenanceRecord } from "../../../lib/api/schemas";
import { useCloseMaintenance, useCreateMaintenance, useUpdateMaintenance, type UpdateFailurePayload } from "../api/maintenance";
import { parseMoney } from "../maintenanceForm";

interface SheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}
interface ItemRef {
  id: string;
  code: string;
  description: string;
}

export function ReportFailureSheet({ item, open, onOpenChange }: SheetProps & { item: ItemRef }) {
  const create = useCreateMaintenance(item.id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Reportar falla" description={`${item.code} · ${item.description}`}>
      <ReportFailureForm
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          await create.mutateAsync({
            kind: "corrective",
            description: values.description.trim(),
            item_status: values.item_status,
            reported_at: dateTimeInputToIso(values.reported_at),
            ...(values.destination.trim() ? { destination: values.destination.trim() } : {}),
          });
          toast.success(`Falla reportada en ${item.code}.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function UpdateFailureSheet({ record, open, onOpenChange }: SheetProps & { record: MaintenanceRecord }) {
  const update = useUpdateMaintenance(record.id, record.item_id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Actualizar reparación" description={`${record.item_code} · ${record.item_description}`}>
      <UpdateFailureForm
        record={record}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const payload: UpdateFailurePayload = {};
          if (values.description.trim() !== record.description) payload.description = values.description.trim();
          if (values.destination.trim() !== (record.destination ?? "")) payload.destination = values.destination.trim() || null;
          const currentStatus = record.item_status === "in_repair" ? "in_repair" : "inoperative";
          if (values.item_status !== currentStatus) payload.item_status = values.item_status;
          if (Object.keys(payload).length === 0) {
            onOpenChange(false);
            return;
          }
          await update.mutateAsync(payload);
          toast.success("Reparación actualizada.");
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function CloseFailureSheet({ record, open, onOpenChange }: SheetProps & { record: MaintenanceRecord }) {
  const close = useCloseMaintenance(record.id, record.item_id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Cerrar falla" description={`${record.item_code} · ${record.item_description}`}>
      <CloseFailureForm
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const cost = parseMoney(values.cost);
          const note = values.resolution_note.trim();
          await close.mutateAsync({
            item_status_after: values.item_status_after,
            ...(cost !== undefined ? { cost } : {}),
            ...(note ? { resolution_note: note } : {}),
          });
          toast.success(`Falla de ${record.item_code} cerrada.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

export function PreventiveSheet({ item, open, onOpenChange }: SheetProps & { item: ItemRef }) {
  const create = useCreateMaintenance(item.id);
  const toast = useToast();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Registrar mantenimiento" description={`${item.code} · ${item.description}`}>
      <PreventiveForm
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const cost = parseMoney(values.cost);
          await create.mutateAsync({
            kind: "preventive",
            description: values.description.trim(),
            performed_at: dateTimeInputToIso(values.performed_at),
            ...(cost !== undefined ? { cost } : {}),
            ...(values.next_due_at ? { next_due_at: startOfDayIso(values.next_due_at) } : {}),
          });
          toast.success(`Mantenimiento registrado en ${item.code}.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}
