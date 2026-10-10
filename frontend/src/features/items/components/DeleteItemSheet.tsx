import { useState } from "react";
import { Button } from "../../../components/atoms/Button";
import { Sheet } from "../../../components/molecules/Sheet";
import { describeError } from "../../../lib/api/errors";
import type { Item } from "../../../lib/api/schemas";
import { useDeleteItem } from "../api/items";

interface DeleteItemSheetProps {
  item: Item;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted: () => void;
}

export function DeleteItemSheet({ item, open, onOpenChange, onDeleted }: DeleteItemSheetProps) {
  const remove = useDeleteItem(item.id);
  const [error, setError] = useState<string | null>(null);

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) setError(null);
        onOpenChange(next);
      }}
      title="Eliminar material"
      description={`${item.code} · ${item.description}`}
    >
      <div className="flex flex-col gap-4">
        <p className="text-sm">
          El material dejará de aparecer en las listas. Su código <strong>{item.code}</strong> quedará reservado.
        </p>
        {error && (
          <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
            {error}
          </p>
        )}
        <div className="flex gap-3">
          <Button className="flex-1" onClick={() => onOpenChange(false)}>
            Cancelar
          </Button>
          <Button
            variant="danger"
            className="flex-1"
            disabled={remove.isPending}
            onClick={async () => {
              setError(null);
              try {
                await remove.mutateAsync();
                onOpenChange(false);
                onDeleted();
              } catch (e) {
                setError(describeError(e));
              }
            }}
          >
            {remove.isPending ? "Eliminando…" : "Eliminar"}
          </Button>
        </div>
      </div>
    </Sheet>
  );
}
