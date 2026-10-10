export function Spinner({ label = "Cargando" }: { label?: string }) {
  return (
    <output className="inline-flex items-center gap-2 text-sm text-ink-soft">
      <span aria-hidden="true" className="size-5 animate-spin rounded-full border-2 border-line border-t-accent" />
      <span>{label}…</span>
    </output>
  );
}
