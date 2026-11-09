import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";

const materialSchema = z.object({ item_id: z.string(), quantity_per_unit: z.number() });
const allocationSchema = z.object({
  item_id: z.string(), required_quantity: z.number(), reserved_quantity: z.number(), delivered_quantity: z.number(),
  returned_quantity: z.number(), deficit_quantity: z.number(),
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
  estimated_days: z.number().int(),
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

export function useTransitionExecution(id: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (action: "start" | "pause" | "resume") => request(`/executions/${id}/${action}`, executionSchema, { method: "POST" }),
    onSuccess: () => client.invalidateQueries({ queryKey: executionsKey }),
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
