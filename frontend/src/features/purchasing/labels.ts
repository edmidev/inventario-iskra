import type { BadgeTone } from "../../components/atoms/Badge";
import type { PurchaseOrderStatus } from "../../lib/api/schemas";

export const orderStatusLabels: Record<PurchaseOrderStatus, string> = {
  draft: "Borrador",
  ordered: "Pedida",
  partially_received: "Recibida parcial",
  received: "Recibida",
  cancelled: "Cancelada",
};

export const orderStatusTones: Record<PurchaseOrderStatus, BadgeTone> = {
  draft: "neutral",
  ordered: "accent",
  partially_received: "warn",
  received: "ok",
  cancelled: "bad",
};

export const orderStatusOptions = (Object.keys(orderStatusLabels) as PurchaseOrderStatus[]).map((value) => ({
  value,
  label: orderStatusLabels[value],
}));
