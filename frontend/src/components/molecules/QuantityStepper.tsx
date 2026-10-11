import clsx from "clsx";
import { Button } from "../atoms/Button";
import { Icon } from "../atoms/Icon";
import { Input } from "../atoms/Input";

interface QuantityStepperProps {
  id?: string;
  /** Text so that the field can be momentarily empty while typing. */
  value: string;
  onChange: (value: string) => void;
  min?: number;
  /** Upper limit (e.g. what a location holds); typing above it clamps. */
  max?: number;
  unit?: string | null;
  /** Quick amounts that set the quantity in one tap. */
  quick?: number[];
  invalid?: boolean;
  "aria-describedby"?: string;
}

const clamp = (n: number, min: number, max = Infinity) => Math.min(max, Math.max(min, Number.isFinite(n) ? Math.trunc(n) : min));

/** Large +/- buttons around a numeric field, sized for thumbs, plus optional quick amounts. */
export function QuantityStepper({ id, value, onChange, min = 0, max, unit, quick = [], invalid, ...aria }: QuantityStepperProps) {
  const current = value.trim() === "" ? min : Number(value);
  const step = (delta: number) => onChange(String(clamp(current + delta, min, max)));
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <Button aria-label="Disminuir cantidad" className="size-14 shrink-0 rounded-2xl" disabled={current <= min} onClick={() => step(-1)}>
          <Icon name="minus" className="size-6" />
        </Button>
        <div className="relative min-w-0 flex-1">
          <Input
            id={id}
            type="number"
            inputMode="numeric"
            min={min}
            max={max}
            step={1}
            value={value}
            invalid={invalid}
            onChange={(event) => {
              const next = event.target.value;
              onChange(max !== undefined && next.trim() !== "" && Number(next) > max ? String(max) : next);
            }}
            className={clsx("min-h-14 text-center text-2xl font-bold tabular-nums", unit && "pr-14")}
            {...aria}
          />
          {unit && <span className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-sm text-ink-soft">{unit}</span>}
        </div>
        <Button aria-label="Aumentar cantidad" className="size-14 shrink-0 rounded-2xl" disabled={max !== undefined && current >= max} onClick={() => step(1)}>
          <Icon name="plus" className="size-6" />
        </Button>
      </div>
      {quick.length > 0 && (
        <fieldset className="flex flex-wrap gap-2">
          <legend className="sr-only">Cantidades rápidas</legend>
          {quick.map((amount) => (
            <Button key={amount} className="min-w-14" aria-pressed={current === amount} onClick={() => onChange(String(amount))}>
              {amount}
            </Button>
          ))}
        </fieldset>
      )}
    </div>
  );
}
