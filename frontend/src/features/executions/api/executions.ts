import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";

const materialSchema = z.object({ item_id: z.string(), quantity_per_unit: z.number() });
const allocationSchema = z.object({
  item_id: z.string(), required_quantity: z.number(), reserved_quantity: z.number(), delivered_quantity: z.number(),
  returned_quantity: z.number(), deficit_quantity: z.number(),
  purchased_pending_quantity: z.number().default(0), unpurchased_quantity: z.number().default(0),
});
const finishedUnitSchema = z.object({
  id: z.number().int(), code: z.string(), execution_id: z.number().int(), produced_at: z.string(),
  delivery_id: z.number().int().optional(), customer_id: z.number().int().optional(), delivered_at: z.string().optional(),
});
const deliveryReceiptSchema = z.object({
  id: z.number().int(), number: z.string(), execution_id: z.number().int(), customer_id: z.number().int(),
  customer_name: z.string(), actor_id: z.string(), unit_codes: z.array(z.string()), delivered_at: z.string(),
});
const finishedStockSchema = z.object({
  id: z.number().int(), code: z.string(), execution_id: z.number().int(), project_name: z.string(), produced_at: z.string(),
  status: z.enum(["available", "delivered"]), receipt: z.string().optional(), customer_name: z.string().optional(), delivered_at: z.string().optional(),
});
const executionEventSchema = z.object({
  id: z.number().int(), execution_id: z.number().int(), type: z.string(), message: z.string(),
  actor_id: z.string().optional(), actor_name: z.string(), occurred_at: z.string(),
});
export const executionSchema = z.object({
  id: z.number(),
  project_id: z.number(),
  project_name: z.string(),
  description: z.string(),
  characteristics: z.array(z.string()),
  consumables: z.array(materialSchema),
  tools: z.array(z.string()),
  template_estimated_days: z.number(),
  units: z.number().int(),
  produced_units: z.number().int().default(0),
  estimated_days: z.number().int(),
  shortage_request_id: z.number().int().positive().optional(),
  materials: z.array(allocationSchema).default([]),
  customer_ids: z.array(z.number()),
  principal_user_id: z.string(),
  worker_ids: z.array(z.string()),
  status: z.enum(["registered", "in_progress", "paused", "completed", "cancelled"]),
  created_at: z.string(),
  updated_at: z.string(),
});
const assignableUserSchema = z.object({ id: z.string(), name: z.string() });
export type Execution = z.infer<typeof executionSchema>;
export type MaterialAllocation = z.infer<typeof allocationSchema>;
export type FinishedUnit = z.infer<typeof finishedUnitSchema>;
export type DeliveryReceipt = z.infer<typeof deliveryReceiptSchema>;
export type FinishedStockRow = z.infer<typeof finishedStockSchema>;
export type ExecutionEvent = z.infer<typeof executionEventSchema>;
export type ExecutionCreate = {
  project_id: number;
  units: number;
  estimated_days?: number;
  customer_ids: number[];
  principal_user_id: string;
  worker_ids: string[];
  additional_materials?: { item_id: string; quantity: number }[];
  additional_tools?: string[];
};

export const executionsKey = ["executions"] as const;
export function useExecutions(q: string) {
  return usePagedQuery([...executionsKey, q], "/executions", executionSchema, { q: q.trim() || undefined });
}

export function useAssignableUsers() {
  return useQuery({
    queryKey: [...executionsKey, "assignable-users"],
    queryFn: ({ signal }) => request("/executions/assignable-users", z.object({ data: z.array(assignableUserSchema) }), { signal }).then((r) => r.data),
  });
}

export function useCreateExecution() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (body: ExecutionCreate) => request("/executions", executionSchema, { method: "POST", body }),
    onSuccess: () => client.invalidateQueries({ queryKey: executionsKey }),
  });
}

export function useAddExecutionMaterial(executionID: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { request_id: string; item_id: string; quantity: number }) =>
      request(`/executions/${executionID}/materials`, executionSchema, { method: "POST", body: input }),
    onSuccess: () => client.invalidateQueries({ queryKey: executionsKey }),
  });
}

export function useAddExecutionTool(executionID: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { request_id: string; item_id: string }) =>
      request(`/executions/${executionID}/tools`, executionSchema, { method: "POST", body: input }),
    onSuccess: () => client.invalidateQueries({ queryKey: executionsKey }),
  });
}

export function useLinkExecutionPurchaseLine(executionID: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { purchase_order_line_id: number; quantity: number }) =>
      request(`/executions/${executionID}/purchase-lines`, executionSchema, { method: "POST", body: input }),
    onSuccess: () => client.invalidateQueries({ queryKey: executionsKey }),
  });
}

export function useTransitionExecution(id: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (action: "start" | "pause" | "resume" | "complete" | "cancel") => request(`/executions/${id}/${action}`, executionSchema, { method: "POST" }),
    onSuccess: () => client.invalidateQueries({ queryKey: executionsKey }),
  });
}

export function useExecutionUnits(executionID: number, enabled: boolean) {
  return useQuery({
    queryKey: [...executionsKey, executionID, "units"], enabled,
    queryFn: ({ signal }) => request(`/executions/${executionID}/units`, z.object({ data: z.array(finishedUnitSchema) }), { signal }).then((r) => r.data),
  });
}

export function useProduceExecution(executionID: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { request_id: string; quantity: number }) =>
      request(`/executions/${executionID}/produce`, z.object({ data: z.array(finishedUnitSchema) }), { method: "POST", body: input }),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: executionsKey });
      void client.invalidateQueries({ queryKey: [...executionsKey, executionID, "units"] });
    },
  });
}

export function useExecutionDeliveries(executionID: number, enabled: boolean) {
  return useQuery({
    queryKey: [...executionsKey, executionID, "deliveries"], enabled,
    queryFn: ({ signal }) => request(`/executions/${executionID}/deliveries`, z.object({ data: z.array(deliveryReceiptSchema) }), { signal }).then((r) => r.data),
  });
}

export function useFinishedStock(status: "" | "available" | "delivered", q: string) {
  return usePagedQuery([...executionsKey, "finished-stock", status, q], "/finished-products", finishedStockSchema, {
    status: status || undefined, q: q.trim() || undefined,
  });
}

export function useExecutionEvents(executionID: number, enabled: boolean) {
  return useQuery({
    queryKey: [...executionsKey, executionID, "events"], enabled,
    queryFn: ({ signal }) => request(`/executions/${executionID}/events`, z.object({ data: z.array(executionEventSchema) }), { signal }).then((response) => response.data),
  });
}

export function useRecordExecutionIncident(executionID: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { request_id: string; type: "positive" | "negative"; message: string }) =>
      request(`/executions/${executionID}/events`, executionEventSchema, { method: "POST", body: input }),
    onSuccess: () => void client.invalidateQueries({ queryKey: [...executionsKey, executionID, "events"] }),
  });
}

export function useDeliverFinishedUnits(executionID: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { request_id: string; customer_id: number; unit_ids: number[] }) =>
      request(`/executions/${executionID}/deliveries`, deliveryReceiptSchema, { method: "POST", body: input }),
    onSuccess: () => void client.invalidateQueries({ queryKey: executionsKey }),
  });
}

export function useMoveExecutionMaterial(executionID: number, itemID: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: { action: "deliver" | "return"; request_id: string; quantity: number; location_id: number; note?: string }) =>
      request(`/executions/${executionID}/materials/${encodeURIComponent(itemID)}/${input.action}`, executionSchema, {
        method: "POST", body: { request_id: input.request_id, quantity: input.quantity, location_id: input.location_id, note: input.note ?? "" },
      }),
    onSuccess: () => client.invalidateQueries({ queryKey: executionsKey }),
  });
}
