import { MaintenanceHistory } from "../../../components/organisms/MaintenanceHistory";
import { describeError } from "../../../lib/api/errors";
import type { Item } from "../../../lib/api/schemas";
import { useItemMaintenance } from "../api/maintenance";

export function MaintenanceTab({ item }: { item: Item }) {
  const query = useItemMaintenance(item.id, true);
  return (
    <MaintenanceHistory
      item={item}
      records={query.data?.pages.flatMap((page) => page.data) ?? []}
      status={query.status}
      errorMessage={query.error ? describeError(query.error) : undefined}
      hasMore={query.hasNextPage}
      isFetchingMore={query.isFetchingNextPage}
      onLoadMore={() => void query.fetchNextPage()}
      onRetry={() => void query.refetch()}
    />
  );
}
