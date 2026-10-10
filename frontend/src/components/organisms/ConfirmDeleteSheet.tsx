import { useState, type ReactNode } from "react";
import { Button } from "../atoms/Button";
import { Sheet } from "../molecules/Sheet";
import { describeError } from "../../lib/api/errors";

interface ConfirmDeleteSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children: ReactNode;
  /** Rejects with an ApiError when the server refuses (e.g. 409 in use); the message is shown in place. */
  onConfirm: () => Promise<void>;
}

export function ConfirmDeleteSheet({ open, onOpenChange, title, description, children, onConfirm }: ConfirmDeleteSheetProps) {
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) setError(null);
        onOpenChange(next);
      }}
      title={title}
      description={description}
    >
      <div className="flex flex-col gap-4">
        <div className="text-sm">{children}</div>
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
            disabled={pending}
            onClick={async () => {
              setError(null);
              setPending(true);
              try {
                await onConfirm();
              } catch (e) {
                setError(describeError(e));
              } finally {
                setPending(false);
              }
            }}
          >
            {pending ? "Eliminando…" : "Eliminar"}
          </Button>
        </div>
      </div>
    </Sheet>
  );
}
