import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { categorySchema, listOf, locationSchema } from "../../../lib/api/schemas";

import { locationPath } from "../locationPath";

const FIVE_MINUTES = 5 * 60 * 1000;

export function useCategories() {
  return useQuery({
    queryKey: ["categories"],
    queryFn: ({ signal }) => request("/categories", listOf(categorySchema), { signal }).then((r) => r.data),
    staleTime: FIVE_MINUTES,
  });
}

export function useLocations() {
  return useQuery({
    queryKey: ["locations"],
    queryFn: ({ signal }) => request("/locations", listOf(locationSchema), { signal }).then((r) => r.data),
    staleTime: FIVE_MINUTES,
  });
}

/** Lookup helpers for rendering category and location names from ids. */
export function useCatalogLookup() {
  const categories = useCategories();
  const locations = useLocations();
  return useMemo(
    () => ({
      categoryName: (id: number) => categories.data?.find((c) => c.id === id)?.name ?? null,
      locationName: (id: number | null) => locationPath(locations.data, id),
      categories: categories.data ?? [],
      locations: locations.data ?? [],
    }),
    [categories.data, locations.data],
  );
}
