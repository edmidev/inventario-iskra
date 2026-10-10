import clsx from "clsx";
import { NavLink } from "react-router-dom";
import { Icon, type IconName } from "./Icon";

interface NavItemProps {
  to: string;
  label: string;
  icon: IconName;
  variant: "bottom" | "side";
}

export function NavItem({ to, label, icon, variant }: NavItemProps) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        clsx(
          "flex items-center text-sm font-medium transition-colors",
          variant === "bottom"
            ? "min-h-14 flex-1 flex-col justify-center gap-0.5 text-xs"
            : "min-h-11 gap-3 rounded-xl px-3",
          isActive
            ? variant === "bottom"
              ? "font-semibold text-accent"
              : "bg-accent-soft font-semibold text-accent"
            : "text-ink-soft hover:text-ink",
        )
      }
    >
      <Icon name={icon} className={variant === "bottom" ? "size-6" : "size-5"} />
      {label}
    </NavLink>
  );
}
