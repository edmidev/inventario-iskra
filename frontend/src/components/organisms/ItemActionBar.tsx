import { useId } from "react";
import { Button } from "../atoms/Button";
import type { ActionId, ItemAction } from "../../features/operations/actions";

interface ItemActionBarProps {
  actions: ItemAction[];
  onSelect: (id: ActionId) => void;
}

export function ItemActionBar({ actions, onSelect }: ItemActionBarProps) {
  const prefix = useId();
  return (
    <section aria-label="Acciones" className="grid grid-cols-2 gap-2 sm:grid-cols-3">
      {actions.map((action) => {
        const reasonId = `${prefix}-${action.id}`;
        return (
          <div key={action.id} className="flex flex-col gap-1">
            <Button
              variant={action.primary && !action.disabledReason ? "primary" : "secondary"}
              className="min-h-12 w-full"
              disabled={Boolean(action.disabledReason)}
              aria-describedby={action.disabledReason ? reasonId : undefined}
              onClick={() => onSelect(action.id)}
            >
              {action.label}
            </Button>
            {action.disabledReason && (
              <p id={reasonId} className="px-1 text-xs text-ink-soft">
                {action.disabledReason}
              </p>
            )}
          </div>
        );
      })}
    </section>
  );
}
