import type { FieldPath, FieldValues, UseFormSetError } from "react-hook-form";
import { ApiError, describeError } from "./api/errors";

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
    for (const [field, message] of Object.entries(error.fields)) {
      if (knownFields.includes(field)) {
        setError(field as FieldPath<T>, { type: "server", message });
      } else {
        unmapped = unmapped ? `${unmapped} ${field}: ${message}` : `${field}: ${message}`;
      }
    }
    return unmapped;
  }
  return describeError(error);
}
