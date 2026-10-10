import { useState } from "react";
import { useForm, type DefaultValues, type FieldPath, type FieldValues, type Resolver } from "react-hook-form";
import { ApiError, describeError } from "./api/errors";
import { applyApiError } from "./forms";

interface ApiFormOptions<T extends FieldValues> {
  resolver: Resolver<T>;
  defaultValues: DefaultValues<T>;
  /** Fields the API may report in a 422; anything else becomes the form-level message. */
  fields: readonly string[];
  /** API error codes that belong on one field (e.g. email_taken -> email). */
  codeFields?: Record<string, FieldPath<T>>;
  onSubmit: (values: T) => Promise<void>;
}

/** react-hook-form wired to the API error model: 422 `fields` and coded conflicts land on the inputs. */
export function useApiForm<T extends FieldValues>({ resolver, defaultValues, fields, codeFields, onSubmit }: ApiFormOptions<T>) {
  const [formError, setFormError] = useState<string | null>(null);
  const form = useForm<T>({ resolver, defaultValues });
  const submit = form.handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      const target = error instanceof ApiError ? codeFields?.[error.code] : undefined;
      if (target && error instanceof ApiError) {
        form.setError(target, { type: "server", message: describeError(error) });
        return;
      }
      setFormError(applyApiError(error, form.setError, fields));
    }
  });
  return { form, submit, formError };
}
