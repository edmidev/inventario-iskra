import { useMutation } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";
import { movementResultSchema, movementSchema, stockAlertSchema } from "../../../lib/api/schemas";
import { stockKey, useInvalidateOperations } from "../../operations/api/invalidate";

export const movementsKey = (itemId: string) => [...stockKey, "movements", itemId] as const;
export const stockAlertsKey = [...stockKey, "alerts"] as const;

export function useMovements(itemId: string) {
  return usePagedQuery(movementsKey(itemId), `/items/${itemId}/movements`, movementSchema);
}

export function useStockAlerts() {
  return usePagedQuery(stockAlertsKey, "/stock/alerts", stockAlertSchema);
}

/** Payload of POST /items/{id}/movements; mirrors the OpenAPI MovementRequest. */
export interface MovementPayload {
  type: "in" | "out" | "adjust" | "transfer";
  qty?: number;
  to_location_id?: number;
  note?: string;
}

export function useRecordMovement(itemId: string) {
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (payload: MovementPayload) =>
      request(`/items/${itemId}/movements`, movementResultSchema, { method: "POST", body: payload }),
    onSuccess: () => invalidate(itemId),
  });
}
