import { MovementForm, TransferForm, type StockSubject } from "../../../components/organisms/StockForms";
import { StockTransferForm } from "../../../components/organisms/StockTransferForm";
import { Sheet } from "../../../components/molecules/Sheet";
import { useToast } from "../../../components/molecules/Toast";
import { useCatalogLookup } from "../../catalog/api/catalog";
import { useRecordMovement } from "../api/stock";
import type { Item } from "../../../lib/api/schemas";
import { movementCopy, toMovementPayload, toStockTransferPayload, type QuantityMovement } from "../movementForm";
import { locationDisplayName, recallLocation, rememberLocation } from "../stocks";

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
  const catalog = useCatalogLookup();
  const copy = movementCopy[mode];
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title={copy.title} description={`${subject.code} · ${subject.description}`}>
      <MovementForm
        mode={mode}
        subject={subject}
        locations={catalog.locations}
        lastLocation={recallLocation(subject.id)}
        onCancel={() => onOpenChange(false)}
        onSubmit={async (values) => {
          const result = await record.mutateAsync(toMovementPayload(mode, values));
          rememberLocation(subject.id, values.location_id);
          const where = locationDisplayName(catalog.locations, [...subject.stocks, ...result.stocks], values.location_id);
          toast.success(`${copy.done}${where ? ` en ${where}` : ""}. Existencias: ${result.stock_current} ${subject.unit ?? ""}`.trim());
          onOpenChange(false);
        }}
      />
    </Sheet>
  );
}

interface TransferSheetProps {
  item: Pick<Item, "id" | "code" | "description" | "kind" | "location_id" | "unit" | "stocks">;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Assets move to another location; consumables move a quantity between two locations. */
export function TransferSheet({ item, open, onOpenChange }: TransferSheetProps) {
  const record = useRecordMovement(item.id);
  const toast = useToast();
  const catalog = useCatalogLookup();
  const consumable = item.kind === "consumable";
  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      title={consumable ? "Trasladar existencias" : "Trasladar material"}
      description={`${item.code} · ${item.description}`}
    >
      {consumable ? (
        <StockTransferForm
          unit={item.unit}
          stocks={item.stocks}
          locations={catalog.locations}
          onCancel={() => onOpenChange(false)}
          onSubmit={async (values) => {
            await record.mutateAsync(toStockTransferPayload(values));
            const from = locationDisplayName(catalog.locations, item.stocks, values.from_location_id);
            const to = locationDisplayName(catalog.locations, item.stocks, values.to_location_id);
            toast.success(`Trasladado: ${values.qty}${item.unit ? ` ${item.unit}` : ""} de ${from} a ${to}.`);
            onOpenChange(false);
          }}
        />
      ) : (
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
      )}
    </Sheet>
  );
}
