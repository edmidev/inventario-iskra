import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { itemPageSchema, itemSchema, type Item } from "../../../lib/api/schemas";
import { dashboardKey } from "../../dashboard/api/useDashboardSummary";
import type { ItemFilters } from "../filters";

export const PAGE_SIZE = 25;
export const itemsKey = ["items"] as const;
export const itemKey = (id: string) => ["item", id] as const;

export function useItems(filters: ItemFilters) {
  return useInfiniteQuery({
    queryKey: [...itemsKey, filters],
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      request("/items", itemPageSchema, {
        signal,
        query: {
          q: filters.q.trim() || undefined,
          category_id: filters.categoryId,
          location_id: filters.locationId,
          status: filters.status,
          kind: filters.kind,
          availability: filters.availability,
          low_stock: filters.lowStock ? true : undefined,
          sort: filters.sort,
          order: filters.order,
          limit: PAGE_SIZE,
          offset: pageParam,
        },
      }),
    getNextPageParam: (last) => (last.offset + last.limit < last.total ? last.offset + last.limit : undefined),
  });
}

export function useItem(id: string) {
  return useQuery({
    queryKey: itemKey(id),
    queryFn: ({ signal }) => request(`/items/${id}`, itemSchema, { signal }),
  });
}

/** Payload accepted by POST /items; mirrors the OpenAPI ItemCreate schema. */
export interface ItemCreatePayload {
  code?: string;
  description: string;
  category_id: number;
  kind: "asset" | "consumable";
  status?: "functional" | "inoperative" | "in_repair";
  unit?: string;
  stock_current?: number;
  stock_min?: number;
  location_id?: number | null;
  maintenance_interval_days?: number | null;
  attributes?: Record<string, unknown>;
  notes?: string;
}

/** Payload accepted by PATCH /items/{id}; mirrors the OpenAPI ItemUpdate schema. */
export type ItemUpdatePayload = Partial<
  Pick<ItemCreatePayload, "description" | "category_id" | "status" | "unit" | "stock_min" | "attributes" | "notes">
> & { location_id?: number | null; maintenance_interval_days?: number | null };

function useInvalidateLists() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: itemsKey }),
      queryClient.invalidateQueries({ queryKey: dashboardKey }),
    ]);
}

export function useCreateItem() {
  const queryClient = useQueryClient();
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: (payload: ItemCreatePayload) => request("/items", itemSchema, { method: "POST", body: payload }),
    onSuccess: async (item: Item) => {
      queryClient.setQueryData(itemKey(item.id), item);
      await invalidate();
    },
  });
}

export function useUpdateItem(id: string) {
  const queryClient = useQueryClient();
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: (payload: ItemUpdatePayload) =>
      request(`/items/${id}`, itemSchema, { method: "PATCH", body: payload }),
    onSuccess: async (item: Item) => {
      queryClient.setQueryData(itemKey(item.id), item);
      await invalidate();
    },
  });
}

export function useDeleteItem(id: string) {
  const queryClient = useQueryClient();
  const invalidate = useInvalidateLists();
  return useMutation({
    mutationFn: () => request(`/items/${id}`, null, { method: "DELETE" }),
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: itemKey(id) });
      await invalidate();
    },
  });
}
