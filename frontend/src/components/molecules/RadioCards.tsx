import type { UseFormRegisterReturn } from "react-hook-form";
import { FieldGroup } from "./FormField";

interface RadioCardsProps {
  legend: string;
  registration: UseFormRegisterReturn;
  options: { value: string; label: string }[];
  error?: string;
}

/** Big tappable single-choice options bound to react-hook-form. */
export function RadioCards({ legend, registration, options, error }: RadioCardsProps) {
  return (
    <FieldGroup legend={legend} error={error}>
      <div className="grid grid-cols-2 gap-2">
        {options.map((option) => (
          <label
            key={option.value}
            className="flex min-h-11 cursor-pointer items-center justify-center rounded-xl border border-line px-2 text-center text-sm font-semibold has-[:checked]:border-accent has-[:checked]:bg-accent-soft has-[:checked]:text-accent has-[:focus-visible]:outline-2"
          >
            <input type="radio" value={option.value} className="sr-only" {...registration} />
            {option.label}
          </label>
        ))}
      </div>
    </FieldGroup>
  );
}
