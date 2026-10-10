import { useMemo } from "react";
import type { Item } from "../../lib/api/schemas";
import { useItemCustody } from "../custody/api/custody";
import { useMovements } from "../stock/api/stock";
import { mergeTimeline } from "./timeline";

/**
 * Movements apply to both kinds (assets have transfers); custody only to assets.
 * Both lists are fetched newest first and merged on the client.
 */
export function useItemTimeline(item: Pick<Item, "id" | "kind">) {
  const isAsset = item.kind === "asset";
  const movements = useMovements(item.id);
  const custody = useItemCustody(item.id, isAsset);

  const timeline = useMemo(
    () =>
      mergeTimeline({
        movements: movements.data?.pages.flatMap((page) => page.data) ?? [],
        movementsHasMore: movements.hasNextPage,
        assignments: custody.data?.pages.flatMap((page) => page.data) ?? [],
        custodyHasMore: isAsset && custody.hasNextPage,
      }),
    [movements.data, movements.hasNextPage, custody.data, custody.hasNextPage, isAsset],
  );

  const pending = movements.isPending || (isAsset && custody.isPending);
  const failed = movements.isError || (isAsset && custody.isError);
  const error = movements.error ?? (isAsset ? custody.error : null);
  const next = timeline.nextStream === "custody" ? custody : movements;

  return {
    ...timeline,
    status: pending ? ("pending" as const) : failed ? ("error" as const) : ("success" as const),
    error,
    isFetchingMore: next.isFetchingNextPage,
    loadMore: () => void next.fetchNextPage(),
    retry: () => void Promise.all([movements.isError && movements.refetch(), custody.isError && custody.refetch()]),
  };
}
