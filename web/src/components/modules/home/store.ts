import { create } from 'zustand';
import { createJSONStorage, persist } from 'zustand/middleware';

/** Unified channel ranking sort (merged rank + performance). */
export type ChannelRankingSortMode =
    | 'count'
    | 'success_rate'
    | 'avg_latency'
    | 'tokens_per_sec'
    | 'cost'
    | 'tokens';

export type ChartMetricType = 'cost' | 'count' | 'tokens';
export type ChartPeriod = '1' | '7' | '30';

const VALID_RANKING_SORT = new Set<string>([
    'count',
    'success_rate',
    'avg_latency',
    'tokens_per_sec',
    'cost',
    'tokens',
]);

/** Returns null when value is missing/invalid so callers can fall back. */
export function normalizeChannelRankingSort(value: unknown): ChannelRankingSortMode | null {
    if (typeof value === 'string' && VALID_RANKING_SORT.has(value)) {
        return value as ChannelRankingSortMode;
    }
    return null;
}

interface HomeViewState {
    channelRankingSort: ChannelRankingSortMode;
    chartMetricType: ChartMetricType;
    chartPeriod: ChartPeriod;
    setChannelRankingSort: (value: ChannelRankingSortMode) => void;
    setChartMetricType: (value: ChartMetricType) => void;
    setChartPeriod: (value: ChartPeriod) => void;
}

export const useHomeViewStore = create<HomeViewState>()(
    persist(
        (set) => ({
            channelRankingSort: 'count',
            chartMetricType: 'cost',
            chartPeriod: '1',
            setChannelRankingSort: (value) => set({ channelRankingSort: value }),
            setChartMetricType: (value) => set({ chartMetricType: value }),
            setChartPeriod: (value) => set({ chartPeriod: value }),
        }),
        {
            name: 'home-view-options-storage',
            storage: createJSONStorage(() => localStorage),
            partialize: (state) => ({
                channelRankingSort: state.channelRankingSort,
                chartMetricType: state.chartMetricType,
                chartPeriod: state.chartPeriod,
            }),
            merge: (persisted, current) => {
                const p = (persisted ?? {}) as Partial<HomeViewState>;
                // Prefer new key; fall back to legacy rankSortMode (cost|count|tokens) for 旧 localStorage 数据
                const legacy = (p as Record<string, unknown>).rankSortMode;
                const ranking =
                    normalizeChannelRankingSort(p.channelRankingSort) ??
                    normalizeChannelRankingSort(legacy) ??
                    'count';
                return {
                    ...current,
                    ...p,
                    channelRankingSort: ranking,
                };
            },
        },
    ),
);
