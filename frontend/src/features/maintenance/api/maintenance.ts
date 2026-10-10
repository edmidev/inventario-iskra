import { useMutation } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";
import { dueMaintenanceSchema, maintenanceRecordSchema } from "../../../lib/api/schemas";
import { maintenanceKey, useInvalidateOperations } from "../../operations/api/invalidate";

export const itemMaintenanceKey = (itemId: string) => [...maintenanceKey, "item", itemId] as const;

export function useItemMaintenance(itemId: string, enabled: boolean) {
  return usePagedQuery(itemMaintenanceKey(itemId), `/items/${itemId}/maintenance`, maintenanceRecordSchema, {}, { enabled });
}

/** The item's open failure, if any (the API allows at most one). */
export function useOpenFailure(itemId: string, enabled: boolean) {
  return usePagedQuery(
    [...maintenanceKey, "open-failure", itemId],
    "/maintenance",
    maintenanceRecordSchema,
    { item_id: itemId, status: "open", kind: "corrective" },
    { enabled, limit: 1 },
  );
}

export function useOpenFailures() {
  return usePagedQuery([...maintenanceKey, "list", "open"], "/maintenance", maintenanceRecordSchema, {
    status: "open",
    kind: "corrective",
  });
}

export function useDueMaintenance(withinDays: number) {
  return usePagedQuery([...maintenanceKey, "due", withinDays], "/maintenance/due", dueMaintenanceSchema, {
    within_days: withinDays,
  });
}

export interface ReportFailurePayload {
  kind: "corrective";
  description: string;
  reported_at?: string;
  destination?: string;
  item_status?: "inoperative" | "in_repair";
}

export interface PreventivePayload {
  kind: "preventive";
  description: string;
  performed_at?: string;
  cost?: number;
  next_due_at?: string;
}

export interface UpdateFailurePayload {
  description?: string;
  destination?: string | null;
  item_status?: "inoperative" | "in_repair";
}

export interface CloseFailurePayload {
  closed_at?: string;
  cost?: number;
  resolution_note?: string;
  item_status_after?: "functional" | "inoperative";
}

export function useCreateMaintenance(itemId: string) {
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (payload: ReportFailurePayload | PreventivePayload) =>
      request(`/items/${itemId}/maintenance`, maintenanceRecordSchema, { method: "POST", body: payload }),
    onSuccess: () => invalidate(itemId),
  });
}

export function useUpdateMaintenance(recordId: number, itemId: string) {
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (payload: UpdateFailurePayload) =>
      request(`/maintenance/${recordId}`, maintenanceRecordSchema, { method: "PATCH", body: payload }),
    onSuccess: () => invalidate(itemId),
  });
}

export function useCloseMaintenance(recordId: number, itemId: string) {
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (payload: CloseFailurePayload) =>
      request(`/maintenance/${recordId}/close`, maintenanceRecordSchema, { method: "POST", body: payload }),
    onSuccess: () => invalidate(itemId),
  });
}
