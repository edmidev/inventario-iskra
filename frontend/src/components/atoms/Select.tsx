import * as RadixSelect from "@radix-ui/react-select";
import clsx from "clsx";
import { Icon } from "./Icon";

export interface SelectOption {
  value: string;
  label: string;
}

interface SelectProps {
  id?: string;
  /** Currently selected value; empty string means "no selection". */
  value: string;
  onValueChange: (value: string) => void;
  options: SelectOption[];
  placeholder: string;
  /** When set, adds a first option that clears the selection. */
  clearLabel?: string;
  invalid?: boolean;
  disabled?: boolean;
  "aria-label"?: string;
  "aria-describedby"?: string;
}

// Radix forbids an empty-string item value, so "cleared" uses a sentinel.
const CLEAR = "__clear__";

export function Select({ value, onValueChange, options, placeholder, clearLabel, invalid, disabled, ...aria }: SelectProps) {
  return (
    <RadixSelect.Root
      value={value === "" ? undefined : value}
      onValueChange={(next) => onValueChange(next === CLEAR ? "" : next)}
      disabled={disabled}
    >
      <RadixSelect.Trigger
        aria-invalid={invalid || undefined}
        className={clsx(
          "flex min-h-11 w-full items-center justify-between gap-2 rounded-xl border bg-surface px-3.5 text-left text-base",
          "disabled:cursor-not-allowed disabled:opacity-60 data-[placeholder]:text-ink-soft",
          invalid ? "border-bad" : "border-line",
        )}
        {...aria}
      >
        <RadixSelect.Value placeholder={placeholder} />
        <RadixSelect.Icon>
          <Icon name="chevron" className="size-4 rotate-90 text-ink-soft" />
        </RadixSelect.Icon>
      </RadixSelect.Trigger>
      <RadixSelect.Portal>
        <RadixSelect.Content
          position="popper"
          sideOffset={4}
          className="z-[60] max-h-72 w-[var(--radix-select-trigger-width)] overflow-hidden rounded-xl border border-line bg-surface shadow-xl"
        >
          <RadixSelect.Viewport className="p-1">
            {clearLabel && <SelectItem value={CLEAR} label={clearLabel} muted />}
            {options.map((option) => (
              <SelectItem key={option.value} {...option} />
            ))}
          </RadixSelect.Viewport>
        </RadixSelect.Content>
      </RadixSelect.Portal>
    </RadixSelect.Root>
  );
}

function SelectItem({ value, label, muted }: SelectOption & { muted?: boolean }) {
  return (
    <RadixSelect.Item
      value={value}
      className={clsx(
        "flex min-h-11 cursor-pointer items-center justify-between rounded-lg px-3 text-base outline-none",
        "data-[highlighted]:bg-sunken data-[state=checked]:font-semibold",
        muted && "text-ink-soft",
      )}
    >
      <RadixSelect.ItemText>{label}</RadixSelect.ItemText>
      <RadixSelect.ItemIndicator>
        <Icon name="check" className="size-4 text-accent" />
      </RadixSelect.ItemIndicator>
    </RadixSelect.Item>
  );
}
