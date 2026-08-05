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

/** @deprecated use ChannelRankingSortMode */
export type RankSortMode = 'cost' | 'count' | 'tokens';

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
    /** @deprecated migrated to channelRankingSort */
    rankSortMode: RankSortMode;
    channelRankingSort: ChannelRankingSortMode;
    chartMetricType: ChartMetricType;
    chartPeriod: ChartPeriod;
    setRankSortMode: (value: RankSortMode) => void;
    setChannelRankingSort: (value: ChannelRankingSortMode) => void;
    setChartMetricType: (value: ChartMetricType) => void;
    setChartPeriod: (value: ChartPeriod) => void;
}

export const useHomeViewStore = create<HomeViewState>()(
    persist(
        (set) => ({
            rankSortMode: 'count',
            channelRankingSort: 'count',
            chartMetricType: 'cost',
            chartPeriod: '1',
            setRankSortMode: (value) =>
                set({
                    rankSortMode: value,
                    channelRankingSort: normalizeChannelRankingSort(value) ?? 'count',
                }),
            setChannelRankingSort: (value) =>
                set({
                    channelRankingSort: value,
                    // keep legacy field in sync when possible
                    rankSortMode:
                        value === 'cost' || value === 'count' || value === 'tokens' ? value : 'count',
                }),
            setChartMetricType: (value) => set({ chartMetricType: value }),
            setChartPeriod: (value) => set({ chartPeriod: value }),
        }),
        {
            name: 'home-view-options-storage',
            storage: createJSONStorage(() => localStorage),
            partialize: (state) => ({
                rankSortMode: state.rankSortMode,
                channelRankingSort: state.channelRankingSort,
                chartMetricType: state.chartMetricType,
                chartPeriod: state.chartPeriod,
            }),
            merge: (persisted, current) => {
                const p = (persisted ?? {}) as Partial<HomeViewState>;
                // Prefer new key; fall back to legacy rankSortMode (cost|count|tokens)
                const ranking =
                    normalizeChannelRankingSort(p.channelRankingSort) ??
                    normalizeChannelRankingSort(p.rankSortMode) ??
                    'count';
                return {
                    ...current,
                    ...p,
                    channelRankingSort: ranking,
                    rankSortMode:
                        ranking === 'cost' || ranking === 'count' || ranking === 'tokens'
                            ? ranking
                            : 'count',
                };
            },
        },
    ),
);
