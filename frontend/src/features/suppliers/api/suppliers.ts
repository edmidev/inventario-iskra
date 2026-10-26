import { useMutation, useQueryClient } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";
import { supplierSchema } from "../../../lib/api/schemas";

export const suppliersKey = ["suppliers"] as const;

export interface SupplierFilters {
  q: string;
  /** "true" | "false" | "" (all). */
  active: string;
}

export function useSuppliers(filters: SupplierFilters) {
  return usePagedQuery([...suppliersKey, "list", filters], "/suppliers", supplierSchema, {
    q: filters.q.trim() || undefined,
    active: filters.active || undefined,
  });
}

/** Active suppliers for pickers (the API caps a page at 100). */
export function useActiveSuppliers() {
  return usePagedQuery([...suppliersKey, "active"], "/suppliers", supplierSchema, { active: true }, { limit: 100 });
}

export interface SupplierPayload {
  name?: string;
  contact_name?: string;
  email?: string;
  phone?: string;
  notes?: string;
  active?: boolean;
}

function useInvalidateSuppliers() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: suppliersKey });
}

export function useCreateSupplier() {
  const invalidate = useInvalidateSuppliers();
  return useMutation({
    mutationFn: (payload: SupplierPayload) => request("/suppliers", supplierSchema, { method: "POST", body: payload }),
    onSuccess: invalidate,
  });
}

export function useUpdateSupplier(id: number) {
  const invalidate = useInvalidateSuppliers();
  return useMutation({
    mutationFn: (payload: SupplierPayload) => request(`/suppliers/${id}`, supplierSchema, { method: "PATCH", body: payload }),
    onSuccess: invalidate,
  });
}

export function useDeleteSupplier(id: number) {
  const invalidate = useInvalidateSuppliers();
  return useMutation({
    mutationFn: () => request(`/suppliers/${id}`, null, { method: "DELETE" }),
    onSuccess: invalidate,
  });
}

/** Every supplier (active or not) for filters; the API caps a page at 100. */
export function useAllSuppliers() {
  return usePagedQuery([...suppliersKey, "all"], "/suppliers", supplierSchema, {}, { limit: 100 });
}
