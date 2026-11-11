import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../../lib/api/errors";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";
import { customerSchema } from "../schemas";
function localizedConflict(error: unknown): never {
  if (error instanceof ApiError && error.code === "customer_name_taken") {
    throw new ApiError(error.status, error.code, "Ya existe un cliente con ese nombre.", error.fields);
  }
  throw error;
}

export const customersKey = ["customers"] as const;
export function useCustomer(id: number, enabled = true) {
  return useQuery({
    queryKey: [...customersKey, "detail", id], enabled: enabled && Number.isInteger(id) && id > 0,
    queryFn: ({ signal }) => request(`/customers/${id}`, customerSchema, { signal }),
  });
}
export interface CustomerFilters {
  q: string;
  /** "true" | "false" | "" (all). */
  active: string;
}
export function useCustomers(filters: CustomerFilters) {
  return usePagedQuery([...customersKey, "list", filters], "/customers", customerSchema, {
    q: filters.q.trim() || undefined,
    active: filters.active || undefined
  });
}

/** Active customers for pickers (the API caps a page at 100). */
export function useActiveCustomers() {
  return usePagedQuery([...customersKey, "active"], "/customers", customerSchema, {
    active: true
  }, {
    limit: 100
  });
}
export interface CustomerPayload {
  name?: string;
  identification?: string;
  contact?: string;
  notes?: string;
  active?: boolean;
}
function useInvalidateCustomers() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({
    queryKey: customersKey
  });
}
export function useCreateCustomer() {
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (payload: CustomerPayload) => request("/customers", customerSchema, {
      method: "POST",
      body: payload
    }).catch(localizedConflict),
    onSuccess: invalidate
  });
}
export function useUpdateCustomer(id: number) {
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: (payload: CustomerPayload) => request(`/customers/${id}`, customerSchema, {
      method: "PATCH",
      body: payload
    }).catch(localizedConflict),
    onSuccess: invalidate
  });
}
export function useDeleteCustomer(id: number) {
  const invalidate = useInvalidateCustomers();
  return useMutation({
    mutationFn: () => request(`/customers/${id}`, null, {
      method: "DELETE"
    }),
    onSuccess: invalidate
  });
}

/** Every customer (active or not) for filters; the API caps a page at 100. */
export function useAllCustomers() {
  return usePagedQuery([...customersKey, "all"], "/customers", customerSchema, {}, {
    limit: 100
  });
}
