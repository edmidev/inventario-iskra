import { useInfiniteQuery, type QueryKey } from "@tanstack/react-query";
import type { z } from "zod";
import { request } from "./client";
import { pageOf } from "./schemas";

export const PAGE_LIMIT = 20;

interface PagedOptions {
  enabled?: boolean;
  limit?: number;
}

/** limit/offset pagination for the list endpoints that answer `{data,total,limit,offset}`. */
export function usePagedQuery<T extends z.ZodType>(
  key: QueryKey,
  path: string,
  schema: T,
  query: Record<string, string | number | boolean | null | undefined> = {},
  { enabled = true, limit = PAGE_LIMIT }: PagedOptions = {},
) {
  const pageSchema = pageOf(schema);
  return useInfiniteQuery({
    queryKey: key,
    enabled,
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) => request(path, pageSchema, { signal, query: { ...query, limit, offset: pageParam } }),
    getNextPageParam: (last) => (last.offset + last.limit < last.total ? last.offset + last.limit : undefined),
  });
}
