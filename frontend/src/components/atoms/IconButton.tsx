import clsx from "clsx";
import type { ButtonHTMLAttributes } from "react";
import { Icon, type IconName } from "./Icon";
import { Tooltip } from "./Tooltip";

interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "children"> {
  icon: IconName;
  /** Accessible name, also shown as a tooltip. */
  label: string;
}

export function IconButton({ icon, label, className, type = "button", ...props }: IconButtonProps) {
  return (
    <Tooltip content={label}>
      <button
        type={type}
        aria-label={label}
        className={clsx(
          "inline-grid size-11 place-items-center rounded-xl text-ink transition-colors hover:bg-sunken",
          "disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        {...props}
      >
        <Icon name={icon} />
      </button>
    </Tooltip>
  );
}
