import { Controller, type Control, type FieldPath, type FieldValues } from "react-hook-form";
import { Select, type SelectOption } from "../atoms/Select";
import { FormField } from "./FormField";

interface ControlledSelectProps<T extends FieldValues> {
  control: Control<T>;
  name: FieldPath<T>;
  label: string;
  placeholder: string;
  options: SelectOption[];
  clearLabel?: string;
  required?: boolean;
  disabled?: boolean;
  error?: string;
  /** Called after the form value changed, e.g. to reset a dependent field. */
  onValueChange?: (value: string) => void;
}

/** A labelled Select bound to react-hook-form; the label stays programmatically linked to the trigger. */
export function ControlledSelect<T extends FieldValues>({ control, name, label, required, error, onValueChange, ...select }: ControlledSelectProps<T>) {
  return (
    <FormField label={label} required={required} error={error}>
      {(aria) => (
        <Controller
          control={control}
          name={name}
          render={({ field }) => <Select {...select} {...aria} value={field.value ?? ""} onValueChange={(next) => {
            field.onChange(next);
            onValueChange?.(next);
          }} />}
        />
      )}
    </FormField>
  );
}
