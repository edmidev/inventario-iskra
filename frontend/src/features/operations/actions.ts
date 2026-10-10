import type { Item, MaintenanceRecord } from "../../lib/api/schemas";

export type ActionId =
  | "in"
  | "out"
  | "adjust"
  | "transfer"
  | "assign"
  | "return"
  | "report-failure"
  | "update-failure"
  | "close-failure"
  | "preventive";

export interface ItemAction {
  id: ActionId;
  label: string;
  primary?: boolean;
  /** When set the action is shown disabled and the reason is displayed under it. */
  disabledReason?: string;
}

/**
 * The actions an operator can take on an item right now. Unavailable ones stay visible with the
 * reason, so nobody wonders where a button went; irrelevant ones (return when not lent) are left out.
 */
export function buildActions(item: Item, openFailure: MaintenanceRecord | null): ItemAction[] {
  if (item.kind === "consumable") {
    return [
      { id: "in", label: "Entrada", primary: true },
      { id: "out", label: "Salida", primary: true },
      { id: "adjust", label: "Ajuste" },
      { id: "transfer", label: "Trasladar" },
    ];
  }
  const actions: ItemAction[] = [];
  if (item.on_loan) {
    actions.push({ id: "return", label: "Devolver", primary: true });
  } else {
    actions.push({
      id: "assign",
      label: "Prestar",
      primary: true,
      disabledReason: item.status !== "functional" ? "Solo se prestan materiales funcionales." : undefined,
    });
  }
  actions.push({
    id: "transfer",
    label: "Trasladar",
    disabledReason: item.on_loan ? "Está prestado: devuélvelo antes de trasladarlo." : undefined,
  });
  if (openFailure) {
    actions.push({ id: "update-failure", label: "Actualizar reparación" }, { id: "close-failure", label: "Cerrar falla", primary: true });
  } else {
    actions.push({ id: "report-failure", label: "Reportar falla" });
  }
  actions.push({ id: "preventive", label: "Registrar mantenimiento" });
  return actions;
}
