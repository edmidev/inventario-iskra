import type { IconName } from "../atoms/Icon";

export const navItems: { to: string; label: string; icon: IconName }[] = [
  { to: "/inicio", label: "Inicio", icon: "home" },
  { to: "/materiales", label: "Materiales", icon: "box" },
  { to: "/escanear", label: "Escanear", icon: "scan" },
  { to: "/mas", label: "Más", icon: "menu" },
];
