import { useMutation } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";
import { assignmentSchema } from "../../../lib/api/schemas";
import { custodyKey, useInvalidateOperations } from "../../operations/api/invalidate";

export const itemCustodyKey = (itemId: string) => [...custodyKey, "item", itemId] as const;

export function useItemCustody(itemId: string, enabled: boolean) {
  return usePagedQuery(itemCustodyKey(itemId), `/items/${itemId}/custody`, assignmentSchema, {}, { enabled });
}

export interface ActiveLoanFilters {
  overdue: boolean;
  assigneeLocationId: number | null;
  q: string;
}

export function useActiveCustody(filters: ActiveLoanFilters) {
  return usePagedQuery([...custodyKey, "active", filters], "/custody/active", assignmentSchema, {
    overdue: filters.overdue ? true : undefined,
    assignee_location_id: filters.assigneeLocationId,
    q: filters.q.trim() || undefined,
  });
}

export interface AssignPayload {
  assignee_location_id: number;
  due_at?: string;
  note?: string;
}

export interface ReturnPayload {
  return_location_id?: number;
  note?: string;
}

export function useAssign(itemId: string) {
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (payload: AssignPayload) =>
      request(`/items/${itemId}/assign`, assignmentSchema, { method: "POST", body: payload }),
    onSuccess: () => invalidate(itemId),
  });
}

export function useReturn(itemId: string) {
  const invalidate = useInvalidateOperations();
  return useMutation({
    mutationFn: (payload: ReturnPayload) =>
      request(`/items/${itemId}/return`, assignmentSchema, { method: "POST", body: payload }),
    onSuccess: () => invalidate(itemId),
  });
}
