import { useQueryClient } from "@tanstack/react-query";
import { dashboardKey } from "../../dashboard/api/useDashboardSummary";
import { itemKey, itemsKey } from "../../items/api/items";

export const stockKey = ["stock"] as const;
export const custodyKey = ["custody"] as const;
export const maintenanceKey = ["maintenance"] as const;

/**
 * Every write that touches an item's stock, custody, status or maintenance can change the item,
 * the lists, the dashboard counters, the alert pages and the item history, so they all refresh together.
 */
export function useInvalidateOperations() {
  const queryClient = useQueryClient();
  return (itemId: string) =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: itemKey(itemId) }),
      queryClient.invalidateQueries({ queryKey: itemsKey }),
      queryClient.invalidateQueries({ queryKey: dashboardKey }),
      queryClient.invalidateQueries({ queryKey: stockKey }),
      queryClient.invalidateQueries({ queryKey: custodyKey }),
      queryClient.invalidateQueries({ queryKey: maintenanceKey }),
    ]);
}
