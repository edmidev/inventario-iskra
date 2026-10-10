import { Skeleton } from "../atoms/Skeleton";

/** Placeholder rows shown while a list loads; announces itself to assistive tech. */
export function ListSkeleton({ label, rows = 4 }: { label: string; rows?: number }) {
  return (
    <output aria-busy="true" aria-label={label} className="flex flex-col gap-3">
      {Array.from({ length: rows }, (_, index) => (
        <Skeleton key={index} className="h-16" />
      ))}
    </output>
  );
}
