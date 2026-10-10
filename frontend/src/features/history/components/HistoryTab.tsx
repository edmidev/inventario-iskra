import { HistoryTimeline } from "../../../components/organisms/HistoryTimeline";
import { describeError } from "../../../lib/api/errors";
import type { Item } from "../../../lib/api/schemas";
import { useCatalogLookup } from "../../catalog/api/catalog";
import { useItemTimeline } from "../useItemTimeline";

export function HistoryTab({ item }: { item: Item }) {
  const timeline = useItemTimeline(item);
  const catalog = useCatalogLookup();
  return (
    <HistoryTimeline
      events={timeline.events}
      status={timeline.status}
      errorMessage={timeline.error ? describeError(timeline.error) : undefined}
      unit={item.unit}
      hasMore={timeline.hasMore}
      isFetchingMore={timeline.isFetchingMore}
      locationName={catalog.locationName}
      onLoadMore={timeline.loadMore}
      onRetry={timeline.retry}
    />
  );
}
