import { NavItem } from "../atoms/NavItem";

const items = [
  { to: "/inicio", label: "Inicio" },
  { to: "/materiales", label: "Materiales" },
  { to: "/escanear", label: "Escanear" },
  { to: "/mas", label: "Más" },
];

export function BottomNav() {
  return (
    <nav
      aria-label="Navegación principal"
      className="fixed inset-x-0 bottom-0 flex border-t border-slate-200 bg-white pb-[env(safe-area-inset-bottom)]"
    >
      {items.map((item) => (
        <NavItem key={item.to} {...item} />
      ))}
    </nav>
  );
}
