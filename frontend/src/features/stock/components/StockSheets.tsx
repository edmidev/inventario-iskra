import { MovementForm, TransferForm, type StockSubject } from "../../../components/organisms/StockForms";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import { useCatalogLookup } from "../../catalog/api/catalog";
import { useRecordMovement } from "../api/stock";
import { movementCopy, toMovementPayload, type QuantityMovement } from "../movementForm";

export interface MovementSubject extends StockSubject {
  id: string;
  code: string;
  description: string;
}

interface MovementSheetProps {
  mode: QuantityMovement;
  subject: MovementSubject;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Entrada, salida or ajuste of a consumable; confirms with the resulting stock. */
export function MovementSheet({ mode, subject, open, onOpenChange }: MovementSheetProps) {
  const record = useRecordMovement(subject.id);
  const toast = useToast();
  const copy = movementCopy[mode];
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={copy.title} description={`${subject.code} · ${subject.description}`}>
      <MovementForm
        mode={mode}
        subject={subject}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const result = await record.mutateAsync(toMovementPayload(mode, values));
          toast.success(`${copy.done} Existencias: ${result.stock_current} ${subject.unit ?? ""}`.trim());
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

interface TransferSheetProps {
  item: { id: string; code: string; description: string; location_id: number | null };
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function TransferSheet({ item, open, onOpenChange }: TransferSheetProps) {
  const record = useRecordMovement(item.id);
  const toast = useToast();
  const catalog = useCatalogLookup();
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Trasladar material" description={`${item.code} · ${item.description}`}>
      <TransferForm
        locations={catalog.locations}
        currentLocationId={item.location_id}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const to = Number(values.to_location_id);
          const note = values.note.trim();
          await record.mutateAsync({ type: "transfer", to_location_id: to, ...(note ? { note } : {}) });
          toast.success(`Material trasladado a ${catalog.locationName(to) ?? "la nueva ubicación"}.`);
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}
