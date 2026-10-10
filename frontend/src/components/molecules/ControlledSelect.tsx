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
  error?: string;
}

/** A labelled Select bound to react-hook-form; the label stays programmatically linked to the trigger. */
export function ControlledSelect<T extends FieldValues>({ control, name, label, required, error, ...select }: ControlledSelectProps<T>) {
  return (
    <FormField label={label} required={required} error={error}>
      {(aria) => (
        <Controller
          control={control}
          name={name}
          render={({ field }) => <Select {...select} {...aria} value={field.value ?? ""} onValueChange={field.onChange} />}
        />
      )}
    </FormField>
  );
}
