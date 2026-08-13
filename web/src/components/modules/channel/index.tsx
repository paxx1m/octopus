import { useChannelList } from '@/api/endpoints/channel';
import { Card } from './Card';
import { useFilteredList } from '@/hooks/useFilteredList';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';

export function Channel() {
    const { data: channelsData } = useChannelList();

    const { visibleItems, layout } = useFilteredList({
        pageKey: 'channel',
        items: channelsData,
        getName: (c) => c.raw.name,
        getId: (c) => c.raw.id,
        sortField: true,
        extraFilter: (items, filter) => {
            if (filter === 'enabled') return items.filter((c) => c.raw.enabled);
            if (filter === 'disabled') return items.filter((c) => !c.raw.enabled);
            return items;
        },
    });

    return (
        <VirtualizedGrid
            items={visibleItems}
            layout={layout}
            columns={{ default: 1, md: 2, lg: 3 }}
            estimateItemHeight={216}
            getItemKey={(item) => `channel-${item.raw.id}`}
            renderItem={(item) => <Card channel={item.raw} stats={item.formatted} layout={layout} />}
        />
    );
}
