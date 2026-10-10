import { Button } from "../atoms/Button";

interface FormFooterProps {
  onCancel: () => void;
  submitLabel: string;
  pendingLabel: string;
  isSubmitting: boolean;
  danger?: boolean;
  disabled?: boolean;
}

/** Sticky Cancel / Confirm row shared by every action sheet. */
export function FormFooter({ onCancel, submitLabel, pendingLabel, isSubmitting, danger, disabled }: FormFooterProps) {
  return (
    <div className="sticky bottom-0 -mx-5 flex gap-3 border-t border-line bg-surface px-5 pt-3">
      <Button variant="secondary" className="flex-1" onClick={onCancel}>
        Cancelar
      </Button>
      <Button type="submit" variant={danger ? "danger" : "primary"} className="flex-1" disabled={isSubmitting || disabled}>
        {isSubmitting ? pendingLabel : submitLabel}
      </Button>
    </div>
  );
}

export function FormAlert({ message }: { message: string | null }) {
  return message ? (
    <p role="alert" className="rounded-xl bg-bad-soft px-4 py-3 text-sm font-medium text-bad">
      {message}
    </p>
  ) : null;
}
