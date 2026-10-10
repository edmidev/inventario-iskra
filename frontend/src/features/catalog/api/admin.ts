import { useMutation, useQueryClient } from "@tanstack/react-query";
import { request } from "../../../lib/api/client";
import { categorySchema, locationSchema, type Location } from "../../../lib/api/schemas";
import { itemsKey } from "../../items/api/items";

export interface CategoryPayload {
  name?: string;
  code_prefix?: string;
}

export interface LocationPayload {
  name?: string;
  kind?: Location["kind"];
  parent_id?: number | null;
}

/** Catalogs feed every select in the app, and item lists show their names. */
function useRefreshCatalogs() {
  const queryClient = useQueryClient();
  return () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: ["categories"] }),
      queryClient.invalidateQueries({ queryKey: ["locations"] }),
      queryClient.invalidateQueries({ queryKey: itemsKey }),
    ]);
}

export function useCreateCategory() {
  const refresh = useRefreshCatalogs();
  return useMutation({
    mutationFn: (payload: Required<CategoryPayload>) => request("/categories", categorySchema, { method: "POST", body: payload }),
    onSuccess: refresh,
  });
}

export function useUpdateCategory(id: number) {
  const refresh = useRefreshCatalogs();
  return useMutation({
    mutationFn: (payload: CategoryPayload) => request(`/categories/${id}`, categorySchema, { method: "PATCH", body: payload }),
    onSuccess: refresh,
  });
}

export function useDeleteCategory(id: number) {
  const refresh = useRefreshCatalogs();
  return useMutation({
    mutationFn: () => request(`/categories/${id}`, null, { method: "DELETE" }),
    onSuccess: refresh,
  });
}

export function useCreateLocation() {
  const refresh = useRefreshCatalogs();
  return useMutation({
    mutationFn: (payload: Required<Pick<LocationPayload, "name" | "kind">> & Pick<LocationPayload, "parent_id">) =>
      request("/locations", locationSchema, { method: "POST", body: payload }),
    onSuccess: refresh,
  });
}

export function useUpdateLocation(id: number) {
  const refresh = useRefreshCatalogs();
  return useMutation({
    mutationFn: (payload: LocationPayload) => request(`/locations/${id}`, locationSchema, { method: "PATCH", body: payload }),
    onSuccess: refresh,
  });
}

export function useDeleteLocation(id: number) {
  const refresh = useRefreshCatalogs();
  return useMutation({
    mutationFn: () => request(`/locations/${id}`, null, { method: "DELETE" }),
    onSuccess: refresh,
  });
}
