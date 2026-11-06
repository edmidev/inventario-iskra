import { NavItem } from "../atoms/NavItem";
import { visibleNavItems } from "./navItems";

import { usePermissions } from "../../features/auth/session";

export function BottomNav() {
  const { canRead } = usePermissions();
  return (
    <nav
      aria-label="Navegación principal"
      className="fixed inset-x-0 bottom-0 z-30 flex border-t border-line bg-surface pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      {visibleNavItems(canRead)
        .filter((item) => item.mobile !== false)
        .map(({ mobile: _mobile, ...item }) => (
          <NavItem key={item.to} {...item} variant="bottom" />
        ))}
    </nav>
  );
}
