import { useId, type ReactElement, type ReactNode, cloneElement } from "react";

interface FormFieldProps {
  label: string;
  error?: string;
  hint?: string;
  required?: boolean;
  /** A single control, or a render function when the control sits behind a wrapper (e.g. Controller). */
  children: ReactElement<ControlProps> | ((props: ControlProps) => ReactElement);
}

export interface ControlProps {
  id?: string;
  "aria-describedby"?: string;
  invalid?: boolean;
}

export function FormField({ label, error, hint, required, children }: FormFieldProps) {
  const id = useId();
  const describedBy = [error ? `${id}-error` : null, hint ? `${id}-hint` : null].filter(Boolean).join(" ") || undefined;
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className="text-sm font-semibold">
        {label}
        {required && <span aria-hidden="true" className="text-bad"> *</span>}
      </label>
      {typeof children === "function"
        ? children({ id, "aria-describedby": describedBy, invalid: Boolean(error) })
        : cloneElement(children, { id, "aria-describedby": describedBy, invalid: Boolean(error) })}
      {hint && !error && (
        <p id={`${id}-hint`} className="text-xs text-ink-soft">
          {hint}
        </p>
      )}
      {error && (
        <p id={`${id}-error`} role="alert" className="text-sm font-medium text-bad">
          {error}
        </p>
      )}
    </div>
  );
}

export function FieldGroup({ legend, error, children }: { legend: string; error?: string; children: ReactNode }) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-1 text-sm font-semibold">{legend}</legend>
      {children}
      {error && (
        <p role="alert" className="text-sm font-medium text-bad">
          {error}
        </p>
      )}
    </fieldset>
  );
}
