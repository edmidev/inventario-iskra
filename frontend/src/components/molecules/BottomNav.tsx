import { NavItem } from "../atoms/NavItem";
import { navItems } from "./navItems";

export function BottomNav() {
  return (
    <nav
      aria-label="Navegación principal"
      className="fixed inset-x-0 bottom-0 z-30 flex border-t border-line bg-surface pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      {navItems.map((item) => (
        <NavItem key={item.to} {...item} variant="bottom" />
      ))}
    </nav>
  );
}
