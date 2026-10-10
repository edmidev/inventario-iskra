import { useQuery } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { dashboardSummarySchema } from "../../../lib/api/schemas";

export const dashboardKey = ["dashboard", "summary"] as const;

export function useDashboardSummary() {
  return useQuery({
    queryKey: dashboardKey,
    queryFn: ({ signal }) => request("/dashboard/summary", dashboardSummarySchema, { signal }),
  });
}
