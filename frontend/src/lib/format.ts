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

const dateTimeFormat = new Intl.DateTimeFormat("es", {
  day: "2-digit",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? "—" : dateTimeFormat.format(date);
}

// The API does not define a currency, so costs are shown as a plain amount with two decimals.
const moneyFormat = new Intl.NumberFormat("es", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export function formatMoney(value: number | null | undefined): string {
  return value === null || value === undefined ? "—" : `$${moneyFormat.format(value)}`;
}

export const movementLabels = {
  opening: "Existencia inicial",
  in: "Entrada",
  out: "Salida",
  adjust: "Ajuste",
  transfer: "Traslado",
} as const;

export const maintenanceKindLabels = { preventive: "Preventivo", corrective: "Falla" } as const;
