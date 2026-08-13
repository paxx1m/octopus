import { useMemo } from 'react';
import { useSearchStore, useToolbarViewOptionsStore, type ToolbarPage } from '@/components/modules/toolbar';

/**
 * 列表页通用的 sort + filter + search 逻辑。
 *
 * 封装 channel/group/model 三个 index.tsx 中重复的：
 * - useSearchStore(getSearchTerm)
 * - useToolbarViewOptionsStore(getSortField/getSortOrder/getLayout)
 * - useMemo(sort)
 * - useMemo(filter by name + extraFilter)
 *
 * @example
 * const { visibleItems, layout } = useFilteredList({
 *     pageKey: 'channel',
 *     items: channelsData,
 *     getName: (c) => c.raw.name,
 *     getId: (c) => c.raw.id,
 *     extraFilter: (items, filter) => filter === 'enabled' ? items.filter(c => c.raw.enabled) : items,
 * });
 */
export function useFilteredList<T>(opts: {
    pageKey: ToolbarPage;
    items: T[] | undefined;
    getName: (item: T) => string;
    getId?: (item: T) => number;
    /** 排序字段；不传则只按 name 排序 */
    sortField?: boolean;
    /** 额外过滤逻辑，接收已按名称过滤的列表和当前 filter 值 */
    extraFilter?: (items: T[], filter: string) => T[];
}) {
    const searchTerm = useSearchStore((s) => s.getSearchTerm(opts.pageKey));
    const layout = useToolbarViewOptionsStore((s) => s.getLayout(opts.pageKey));
    // getSortField 仅 channel/group 支持，model 永远按 name 排序
    const sortField = useToolbarViewOptionsStore((s) =>
        opts.pageKey === 'channel' || opts.pageKey === 'group' ? s.getSortField(opts.pageKey) : 'name'
    );
    const sortOrder = useToolbarViewOptionsStore((s) => s.getSortOrder(opts.pageKey));
    const filter = useToolbarViewOptionsStore((s) => {
        switch (opts.pageKey) {
            case 'channel': return s.channelFilter;
            case 'group': return s.groupFilter;
            case 'model': return s.modelFilter;
            default: return undefined;
        }
    });

    const sortedItems = useMemo(() => {
        if (!opts.items) return [];
        return [...opts.items].sort((a, b) => {
            let diff: number;
            if (opts.sortField && opts.getId && sortField !== 'name') {
                diff = opts.getId(a) - opts.getId(b);
            } else {
                diff = opts.getName(a).localeCompare(opts.getName(b));
            }
            return sortOrder === 'asc' ? diff : -diff;
        });
        // opts 是引用，其属性在依赖中已展开；sortField/sortOrder 变化时重新排序
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [opts.items, opts.getName, opts.getId, opts.sortField, sortField, sortOrder]);

    const visibleItems = useMemo(() => {
        const term = searchTerm.toLowerCase().trim();
        const byName = !term
            ? sortedItems
            : sortedItems.filter((item) => opts.getName(item).toLowerCase().includes(term));
        if (opts.extraFilter && filter) {
            return opts.extraFilter(byName, filter);
        }
        return byName;
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [sortedItems, searchTerm, opts.getName, opts.extraFilter, filter]);

    return { visibleItems, layout };
}
