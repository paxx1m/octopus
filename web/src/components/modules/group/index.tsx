import { GroupCard } from './Card';
import { useGroupList } from '@/api/endpoints/group';
import { useFilteredList } from '@/hooks/useFilteredList';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';

export function Group() {
    const { data: groups } = useGroupList();

    const { visibleItems } = useFilteredList({
        pageKey: 'group',
        items: groups,
        getName: (g) => g.name,
        getId: (g) => g.id || 0,
        sortField: true,
        extraFilter: (items, filter) => {
            if (filter === 'with-members') return items.filter((g) => (g.items?.length || 0) > 0);
            if (filter === 'empty') return items.filter((g) => (g.items?.length || 0) === 0);
            return items;
        },
    });

    return (
        <VirtualizedGrid
            items={visibleItems}
            columns={{ default: 1, md: 2, lg: 3 }}
            estimateItemHeight={520}
            getItemKey={(group, index) => group.id ?? `group-${index}`}
            renderItem={(group) => <GroupCard group={group} />}
        />
    );
}
