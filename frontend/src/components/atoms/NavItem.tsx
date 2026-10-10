import { NavLink } from "react-router-dom";

type NavItemProps = { to: string; label: string };

export function NavItem({ to, label }: NavItemProps) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        `flex flex-1 flex-col items-center justify-center py-3 text-sm ${
          isActive ? "font-semibold text-blue-600" : "text-slate-500"
        }`
      }
    >
      {label}
    </NavLink>
  );
}
