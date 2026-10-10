import { Icon } from "../atoms/Icon";

export function FilterChip({ label, onRemove }: { label: string; onRemove: () => void }) {
  return (
    <button
      type="button"
      onClick={onRemove}
      aria-label={`Quitar filtro: ${label}`}
      className="inline-flex min-h-9 items-center gap-1.5 rounded-full bg-accent-soft px-3 text-sm font-semibold text-accent"
    >
      {label}
      <Icon name="x" className="size-3.5" />
    </button>
  );
}
