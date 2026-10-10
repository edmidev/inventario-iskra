const dateFormat = new Intl.DateTimeFormat("es", { day: "2-digit", month: "short", year: "numeric" });

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? "—" : dateFormat.format(date);
}

export const statusLabels = {
  functional: "Funcional",
  inoperative: "Inoperativo",
  in_repair: "En reparación",
} as const;

export const kindLabels = { asset: "Activo", consumable: "Consumible" } as const;
export const availabilityLabels = { available: "Disponible", unavailable: "No disponible" } as const;
