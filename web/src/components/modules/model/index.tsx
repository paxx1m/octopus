import { useModelList } from '@/api/endpoints/model';
import { ModelItem } from './Item';
import { useFilteredList } from '@/hooks/useFilteredList';
import { VirtualizedGrid } from '@/components/common/VirtualizedGrid';

export function Model() {
    const { data: models } = useModelList();

    const { visibleItems, layout } = useFilteredList({
        pageKey: 'model',
        items: models,
        getName: (m) => m.name,
        extraFilter: (items, filter) => {
            const hasPricing = (m: typeof items[number]) =>
                m.input + m.output + m.cache_read + m.cache_write > 0;
            if (filter === 'priced') return items.filter(hasPricing);
            if (filter === 'free') return items.filter((m) => !hasPricing(m));
            return items;
        },
    });

    return (
        <VirtualizedGrid
            items={visibleItems}
            layout={layout}
            columns={{ default: 1, md: 2, lg: 3 }}
            estimateItemHeight={112}
            getItemKey={(model) => `model-${model.name}`}
            renderItem={(model) => <ModelItem model={model} layout={layout} />}
        />
    );
}
