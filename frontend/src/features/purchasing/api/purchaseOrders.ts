import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { usePagedQuery } from "../../../lib/api/paged";
import { purchaseOrderSchema, receiveResultSchema, type Currency, type PurchaseOrder, type PurchaseOrderStatus } from "../../../lib/api/schemas";
import { useInvalidateOperations } from "../../operations/api/invalidate";

export const purchaseOrdersKey = ["purchase-orders"] as const;
export const purchaseOrderKey = (id: number) => [...purchaseOrdersKey, "detail", id] as const;

export interface PurchaseOrderFilters {
  status: string;
  supplierId: string;
  q: string;
}

export function usePurchaseOrders(filters: PurchaseOrderFilters) {
  return usePagedQuery([...purchaseOrdersKey, "list", filters], "/purchase-orders", purchaseOrderSchema, {
    status: filters.status || undefined,
    supplier_id: filters.supplierId || undefined,
    q: filters.q.trim() || undefined,
  });
}

export function usePurchaseOrder(id: number) {
  return useQuery({
    queryKey: purchaseOrderKey(id),
    queryFn: ({ signal }) => request(`/purchase-orders/${id}`, purchaseOrderSchema, { signal }),
  });
}

export interface LinePayload {
  item_id?: string;
  new_item?: {
    description: string;
    category_id: number;
    kind: "asset" | "consumable";
    unit?: string;
    stock_min?: number;
  };
  quantity: number;
  /** Integer minor units of the order currency. */
  unit_cost_minor: number;
}

export interface OrderPayload {
  supplier_id: number;
  currency: Currency;
  exchange_rate: number | null;
  expected_at: string | null;
  notes: string;
  lines: LinePayload[];
}

export interface ReceivePayload {
  location_id: number;
  note?: string;
  client_key: string;
  lines: { line_id: number; quantity: number }[];
}

export type { PurchaseOrderStatus };

function useRefreshOrders() {
  const queryClient = useQueryClient();
  return async (order: PurchaseOrder) => {
    queryClient.setQueryData(purchaseOrderKey(order.id), order);
    await queryClient.invalidateQueries({ queryKey: purchaseOrdersKey });
  };
}

export function useCreatePurchaseOrder() {
  const refresh = useRefreshOrders();
  return useMutation({
    mutationFn: (payload: OrderPayload) => request("/purchase-orders", purchaseOrderSchema, { method: "POST", body: payload }),
    onSuccess: refresh,
  });
}

export function useUpdatePurchaseOrder(id: number) {
  const refresh = useRefreshOrders();
  return useMutation({
    mutationFn: (payload: OrderPayload) => request(`/purchase-orders/${id}`, purchaseOrderSchema, { method: "PATCH", body: payload }),
    onSuccess: refresh,
  });
}

export function usePlacePurchaseOrder(id: number) {
  const refresh = useRefreshOrders();
  return useMutation({
    mutationFn: () => request(`/purchase-orders/${id}/order`, purchaseOrderSchema, { method: "POST" }),
    onSuccess: refresh,
  });
}

export function useCancelPurchaseOrder(id: number) {
  const refresh = useRefreshOrders();
  return useMutation({
    mutationFn: () => request(`/purchase-orders/${id}/cancel`, purchaseOrderSchema, { method: "POST" }),
    onSuccess: refresh,
  });
}

/** Receiving creates items and stock movements, so everything stock-related refreshes too. */
export function useReceivePurchaseOrder(id: number) {
  const refresh = useRefreshOrders();
  const invalidateOperations = useInvalidateOperations();
  return useMutation({
    mutationFn: (payload: ReceivePayload) =>
      request(`/purchase-orders/${id}/receive`, receiveResultSchema, { method: "POST", body: payload }),
    onSuccess: async ({ order, receipt }) => {
      const itemIds = new Set<string>();
      for (const line of receipt.lines) for (const received of line.items) itemIds.add(received.item_id);
      for (const line of order.lines) if (line.item_id) itemIds.add(line.item_id);
      await Promise.all([refresh(order), ...[...itemIds].map((itemId) => invalidateOperations(itemId))]);
    },
  });
}
