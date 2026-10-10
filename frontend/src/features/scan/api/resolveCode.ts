import { useQuery } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { itemSchema, type Item } from "../../../lib/api/schemas";

export function fetchItemByCode(code: string, signal?: AbortSignal): Promise<Item> {
  return request(`/items/by-code/${encodeURIComponent(code)}`, itemSchema, { signal });
}

export function useItemByCode(code: string) {
  return useQuery({
    queryKey: ["item-by-code", code],
    queryFn: ({ signal }) => fetchItemByCode(code, signal),
    // A 404 is a definitive answer for a scan; do not hammer the API.
    retry: false,
  });
}
