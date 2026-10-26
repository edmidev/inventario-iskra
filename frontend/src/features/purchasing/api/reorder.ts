import { useQuery } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { reorderSuggestionsSchema } from "../../../lib/api/schemas";
import { dashboardKey } from "../../dashboard/api/useDashboardSummary";

// Nested under the dashboard key so every operation that refreshes the
// dashboard (receiving an order, stock movements) refreshes the suggestions.
export const reorderKey = [...dashboardKey, "reorder"] as const;

export function useReorderSuggestions() {
  return useQuery({
    queryKey: reorderKey,
    queryFn: ({ signal }) => request("/dashboard/reorder-suggestions", reorderSuggestionsSchema, { signal }),
  });
}
