import type { ReactNode } from "react";
import { Button } from "../atoms/Button";
import { Spinner } from "../atoms/Spinner";

export function LoadingState({ label = "Cargando" }: { label?: string }) {
  return (
    <div className="grid place-items-center py-12">
      <Spinner label={label} />
    </div>
  );
}

export function EmptyState({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-2 rounded-card border border-dashed border-line px-6 py-10 text-center">
      <p className="font-semibold">{title}</p>
      {description && <p className="max-w-sm text-sm text-ink-soft">{description}</p>}
      {action}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" className="flex flex-col items-center gap-3 rounded-card bg-bad-soft px-6 py-8 text-center text-bad">
      <p className="font-semibold">{message}</p>
      {onRetry && (
        <Button variant="secondary" onClick={onRetry}>
          Reintentar
        </Button>
      )}
    </div>
  );
}
