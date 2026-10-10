import { Outlet } from "react-router-dom";
import { Icon } from "../atoms/Icon";
import { NavItem } from "../atoms/NavItem";
import { BottomNav } from "../molecules/BottomNav";
import { navItems } from "../molecules/navItems";
import { useSession } from "../../features/auth/session";

function Sidebar() {
  const { state, signOut } = useSession();
  return (
    <aside className="fixed inset-y-0 left-0 hidden w-60 flex-col gap-4 border-r border-line bg-surface p-4 md:flex">
      <p className="px-3 pt-2 text-xl font-bold">Inventario</p>
      <nav aria-label="Navegación principal" className="flex flex-col gap-1">
        {navItems.map((item) => (
          <NavItem key={item.to} {...item} variant="side" />
        ))}
      </nav>
      <div className="mt-auto flex flex-col gap-2 border-t border-line pt-3">
        {state.user && (
          <div className="px-3 text-sm">
            <p className="font-semibold">{state.user.name}</p>
            <p className="truncate text-ink-soft">{state.user.email}</p>
          </div>
        )}
        <button
          type="button"
          onClick={() => void signOut()}
          className="flex min-h-11 items-center gap-3 rounded-xl px-3 text-sm font-medium text-ink-soft hover:text-ink"
        >
          <Icon name="logout" /> Cerrar sesión
        </button>
      </div>
    </aside>
  );
}

export function AppShell() {
  return (
    <div className="min-h-dvh bg-canvas text-ink md:pl-60">
      <Sidebar />
      <main className="mx-auto max-w-5xl px-4 pt-[max(1rem,env(safe-area-inset-top))] pb-28 md:px-8 md:pt-8 md:pb-10">
        <Outlet />
      </main>
      <BottomNav />
    </div>
  );
}
