import { useState } from "react";
import { useForm, type DefaultValues, type FieldPath, type FieldValues, type Resolver } from "react-hook-form";
import { ApiError, describeError } from "./api/errors";
import { applyApiError } from "./forms";

interface ApiFormOptions<TIn extends FieldValues, TOut> {
  resolver: Resolver<TIn, unknown, TOut>;
  defaultValues: DefaultValues<TIn>;
  /** Fields the API may report in a 422; anything else becomes the form-level message. */
  fields: readonly string[];
  /** API error codes that belong on one field (e.g. email_taken -> email). */
  codeFields?: Record<string, FieldPath<TIn>>;
  onSubmit: (values: TOut) => Promise<void>;
}

/** react-hook-form wired to the API error model: 422 `fields` and coded conflicts land on the inputs. */
export function useApiForm<TIn extends FieldValues, TOut = TIn>({
  resolver,
  defaultValues,
  fields,
  codeFields,
  onSubmit,
}: ApiFormOptions<TIn, TOut>) {
  const [formError, setFormError] = useState<string | null>(null);
  const form = useForm<TIn, unknown, TOut>({ resolver, defaultValues });
  const submit = form.handleSubmit(async (values) => {
    setFormError(null);
    try {
      await onSubmit(values);
    } catch (error) {
      const target = error instanceof ApiError ? codeFields?.[error.code] : undefined;
      if (target) {
        form.setError(target, { type: "server", message: describeError(error) });
        return;
      }
      setFormError(applyApiError(error, form.setError, fields));
    }
  });
  return { form, submit, formError };
}
