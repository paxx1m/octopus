import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../client';
import { formatCount, formatMoney, formatTime } from '@/lib/utils';
import {
    avgLatencyMs,
    formatAvgLatency,
    formatSuccessRate,
    formatTokensPerSec,
    successRatePercent,
    tokensPerSecond,
} from '@/lib/metrics';

/**
 * 统计数据
 */
interface StatsMetrics {
    input_token: number;
    output_token: number;
    input_cost: number;
    output_cost: number;
    wait_time: number;
    /** ms, cumulative generation duration after first token (excludes TTFT) */
    output_time?: number;
    request_success: number;
    request_failed: number;
}

export interface StatsDerivedMetrics {
    /** 0–100 */
    success_rate: number;
    success_rate_label: string;
    /** average wait/latency ms */
    avg_latency_ms: number;
    avg_latency_label: string;
    /** output tokens per second over cumulative output_time */
    tokens_per_sec: number;
    tokens_per_sec_label: string;
}

export interface StatsMetricsFormatted extends StatsDerivedMetrics {
    input_token: ReturnType<typeof formatCount>;
    output_token: ReturnType<typeof formatCount>;
    input_cost: ReturnType<typeof formatMoney>;
    output_cost: ReturnType<typeof formatMoney>;
    wait_time: ReturnType<typeof formatTime>;
    request_success: ReturnType<typeof formatCount>;
    request_failed: ReturnType<typeof formatCount>;

    request_count: ReturnType<typeof formatCount>;
    total_token: ReturnType<typeof formatCount>;
    total_cost: ReturnType<typeof formatMoney>;
}

/** Derive rates from raw cumulative StatsMetrics (shared by all formatters). */
export function deriveStatsMetrics(item: StatsMetrics): StatsDerivedMetrics {
    const success = item.request_success ?? 0;
    const failed = item.request_failed ?? 0;
    const wait = item.wait_time ?? 0;
    const output = item.output_token ?? 0;
    const outputTime = item.output_time ?? 0;
    const rate = successRatePercent(success, failed);
    const avgMs = avgLatencyMs(wait, success, failed);
    // output_time is already the post-TTFT generation window (or full duration for non-stream).
    const tps = outputTime > 0 ? tokensPerSecond(output, outputTime) : 0;
    return {
        success_rate: rate,
        success_rate_label: formatSuccessRate(rate),
        avg_latency_ms: avgMs,
        avg_latency_label: formatAvgLatency(avgMs),
        tokens_per_sec: tps,
        tokens_per_sec_label: formatTokensPerSec(tps),
    };
}

export function formatStatsMetrics(item: StatsMetrics): StatsMetricsFormatted {
    return {
        input_token: formatCount(item.input_token),
        output_token: formatCount(item.output_token),
        total_token: formatCount((item.input_token ?? 0) + (item.output_token ?? 0)),
        input_cost: formatMoney(item.input_cost),
        output_cost: formatMoney(item.output_cost),
        total_cost: formatMoney((item.input_cost ?? 0) + (item.output_cost ?? 0)),
        wait_time: formatTime(item.wait_time),
        request_success: formatCount(item.request_success),
        request_failed: formatCount(item.request_failed),
        request_count: formatCount((item.request_success ?? 0) + (item.request_failed ?? 0)),
        ...deriveStatsMetrics(item),
    };
}

export interface StatsChannel extends StatsMetrics {
    channel_id: number;
}

export interface StatsDaily extends StatsMetrics {
    date: string;
}
export interface StatsDailyFormatted extends StatsMetricsFormatted {
    date: string;
}

export interface StatsTotal extends StatsMetrics {
    id: number;
}
export type StatsTotalFormatted = StatsMetricsFormatted;

export interface StatsHourly extends StatsMetrics {
    hour: number;
    date: string;
}
export interface StatsHourlyFormatted extends StatsMetricsFormatted {
    hour: number;
    date: string;
}
/**
 * API Key 统计数据
 */
export interface StatsAPIKey extends StatsMetrics {
    api_key_id: number;
}

export interface StatsAPIKeyFormatted extends StatsMetricsFormatted {
    api_key_id: number;
}
/**
 * 获取今日统计数据 Hook
 */
export function useStatsToday() {
    return useQuery({
        queryKey: ['stats', 'today'],
        queryFn: async () => {
            return apiClient.get<StatsDaily>('/api/v1/stats/today');
        },
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

/**
 * 获取每日统计数据 Hook
 */
export function useStatsDaily() {
    return useQuery({
        queryKey: ['stats', 'daily'],
        queryFn: async () => {
            return apiClient.get<StatsDaily[]>('/api/v1/stats/daily');
        },
        select: (data) => data.map((item): StatsDailyFormatted => ({
            ...formatStatsMetrics(item),
            date: item.date,
        })),
        refetchInterval: 3600000, // 1 小时
        refetchOnMount: 'always',
    });
}
/**
 * 获取总统计数据 Hook
 */
export function useStatsHourly() {
    return useQuery({
        queryKey: ['stats', 'hourly'],
        queryFn: async () => {
            return apiClient.get<StatsHourly[]>('/api/v1/stats/hourly');
        },
        select: (data) => data.map((item): StatsHourlyFormatted => ({
            ...formatStatsMetrics(item),
            hour: item.hour,
            date: item.date,
        })),
        refetchInterval: 10000,// 10 秒
        refetchOnMount: 'always',
    });
}

export function useStatsTotal() {
    return useQuery({
        queryKey: ['stats', 'total'],
        queryFn: async () => {
            return apiClient.get<StatsTotal>('/api/v1/stats/total');
        },
        select: (data) => formatStatsMetrics(data),
        refetchInterval: 10000,// 10 秒
        refetchOnMount: 'always',
    });
}



/**
 * 获取 API Key 统计数据列表 Hook
 */
export function useStatsAPIKey() {
    return useQuery({
        queryKey: ['stats', 'apikey'],
        queryFn: async () => {
            return apiClient.get<StatsAPIKey[]>('/api/v1/stats/apikey');
        },
        select: (data) => data.map((item): StatsAPIKeyFormatted => ({
            ...formatStatsMetrics(item),
            api_key_id: item.api_key_id,
        })),
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}