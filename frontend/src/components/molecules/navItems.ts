import type { IconName } from "../atoms/Icon";

/** `mobile: false` keeps an entry in the sidebar but out of the bottom bar, which has room for five. */
export const navItems: { to: string; label: string; icon: IconName; mobile?: boolean }[] = [
  { to: "/inicio", label: "Inicio", icon: "home" },
  { to: "/materiales", label: "Materiales", icon: "box" },
  { to: "/escanear", label: "Escanear", icon: "scan" },
  { to: "/proveedores", label: "Proveedores", icon: "truck", mobile: false },
  { to: "/ordenes-compra", label: "Órdenes de compra", icon: "cart" },
  { to: "/mas", label: "Más", icon: "menu" },
];
