import type { FieldPath, FieldValues, UseFormSetError } from "react-hook-form";
import { ApiError, describeError } from "./api/errors";

/** `stocks[2].quantity` -> `stocks.2.quantity` (the path react-hook-form uses). */
const toFormPath = (field: string) => field.replace(/\[(\d+)\]/g, ".$1");

/** A known field is an exact name or a pattern whose `*` stands for one array index (`stocks.*.quantity`). */
function isKnown(knownFields: readonly string[], path: string): boolean {
  return knownFields.some((known) => {
    if (known === path) return true;
    if (!known.includes("*")) return false;
    const pattern = known.split("*").map((part) => part.replace(/[.\\]/g, "\\$&")).join("\\d+");
    return new RegExp(`^${pattern}$`).test(path);
  });
}

/**
 * Maps an API error onto a react-hook-form instance: 422 `fields` become field errors,
 * everything else becomes a form-level message. Returns the form-level message (if any).
 */
export function applyApiError<T extends FieldValues>(
  error: unknown,
  setError: UseFormSetError<T>,
  knownFields: readonly string[],
): string | null {
  if (error instanceof ApiError && Object.keys(error.fields).length > 0) {
    let unmapped: string | null = null;
    for (const [rawField, message] of Object.entries(error.fields)) {
      const field = toFormPath(rawField);
      if (isKnown(knownFields, field)) {
        setError(field as FieldPath<T>, { type: "server", message });
      } else {
        unmapped = unmapped ? `${unmapped} ${rawField}: ${message}` : `${rawField}: ${message}`;
      }
    }
    return unmapped;
  }
  return describeError(error);
}
