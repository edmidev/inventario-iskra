import { availabilitySchema, itemKindSchema, itemStatusSchema } from "../../lib/api/schemas";
import type { Availability, ItemKind, ItemStatus } from "../../lib/api/schemas";

export type SortField = "code" | "description" | "updated_at";
export type SortOrder = "asc" | "desc";

export interface ItemFilters {
  q: string;
  categoryId: number | null;
  locationId: number | null;
  status: ItemStatus | null;
  kind: ItemKind | null;
  availability: Availability | null;
  lowStock: boolean;
  sort: SortField;
  order: SortOrder;
}

export const defaultFilters: ItemFilters = {
  q: "",
  categoryId: null,
  locationId: null,
  status: null,
  kind: null,
  availability: null,
  lowStock: false,
  sort: "code",
  order: "asc",
};

const sortFields: SortField[] = ["code", "description", "updated_at"];

function numberParam(value: string | null): number | null {
  if (value === null || !/^\d+$/.test(value)) return null;
  return Number(value);
}

function enumParam<T>(value: string | null, schema: { safeParse: (v: unknown) => { success: boolean; data?: T } }): T | null {
  if (value === null) return null;
  const parsed = schema.safeParse(value);
  return parsed.success ? (parsed.data as T) : null;
}

/** URL search params are the single source of truth for list filters. */
export function parseFilters(params: URLSearchParams): ItemFilters {
  const sort = params.get("sort") as SortField | null;
  const order = params.get("order");
  return {
    q: params.get("q") ?? "",
    categoryId: numberParam(params.get("category_id")),
    locationId: numberParam(params.get("location_id")),
    status: enumParam(params.get("status"), itemStatusSchema),
    kind: enumParam(params.get("kind"), itemKindSchema),
    availability: enumParam(params.get("availability"), availabilitySchema),
    lowStock: params.get("low_stock") === "true",
    sort: sort && sortFields.includes(sort) ? sort : defaultFilters.sort,
    order: order === "desc" ? "desc" : "asc",
  };
}

/** Serializes only non-default values so URLs stay short and shareable. */
export function toSearchParams(filters: ItemFilters): URLSearchParams {
  const params = new URLSearchParams();
  if (filters.q.trim()) params.set("q", filters.q.trim());
  if (filters.categoryId !== null) params.set("category_id", String(filters.categoryId));
  if (filters.locationId !== null) params.set("location_id", String(filters.locationId));
  if (filters.status) params.set("status", filters.status);
  if (filters.kind) params.set("kind", filters.kind);
  if (filters.availability) params.set("availability", filters.availability);
  if (filters.lowStock) params.set("low_stock", "true");
  if (filters.sort !== defaultFilters.sort) params.set("sort", filters.sort);
  if (filters.order !== defaultFilters.order) params.set("order", filters.order);
  return params;
}

export function activeFilterCount(filters: ItemFilters): number {
  return [
    filters.categoryId !== null,
    filters.locationId !== null,
    filters.status !== null,
    filters.kind !== null,
    filters.availability !== null,
    filters.lowStock,
  ].filter(Boolean).length;
}
