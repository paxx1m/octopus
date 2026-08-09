import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiClient, API_BASE_URL } from '../client';
import { logger } from '@/lib/logger';

export type HealthStatus =
    | 'disabled'
    | 'circuit_open'
    | 'rate_limited'
    | 'circuit_half_open'
    | 'degraded'
    | 'ok';

export interface HealthRateLimit {
    cooldown_sec: number;
    cooldown_until: number;
    remaining_sec: number;
}

export interface HealthCircuit {
    state: 'closed' | 'open' | 'half_open';
    consecutive_failures: number;
    trip_count: number;
    cooldown_until?: number;
    remaining_sec?: number;
    probe_pending?: boolean;
}

export interface HealthModel {
    name: string;
    status: HealthStatus;
    circuit?: HealthCircuit;
}

export interface HealthKey {
    id: number;
    masked: string;
    remark?: string;
    enabled: boolean;
    status: HealthStatus;
    status_code: number;
    rate_limit?: HealthRateLimit;
    models?: HealthModel[];
}

export interface HealthChannel {
    id: number;
    name: string;
    enabled: boolean;
    allow_empty_key: boolean;
    status: HealthStatus;
    available_keys: number;
    total_keys: number;
    group_ids?: number[];
    keys?: HealthKey[];
}

export interface HealthSummary {
    channels_total: number;
    channels_abnormal: number;
    disabled: number;
    circuit_open: number;
    rate_limited: number;
    half_open: number;
    degraded: number;
    ok: number;
}

export interface HealthSnapshot {
    summary: HealthSummary;
    channels: HealthChannel[];
    ts: number;
}

export type HealthFilterParams = {
    groupId?: number;
    abnormalOnly?: boolean;
};

/** Full snapshot query — no server-side filter; filtering is client-side. */
const FULL_LIST_QS = 'abnormal_only=0';
export const healthListQueryKey = ['health', 'list', 'full'] as const;

function emptySummary(): HealthSummary {
    return {
        channels_total: 0,
        channels_abnormal: 0,
        disabled: 0,
        circuit_open: 0,
        rate_limited: 0,
        half_open: 0,
        degraded: 0,
        ok: 0,
    };
}

async function fetchHealthListFull(): Promise<HealthSnapshot> {
    const data = await apiClient.get<HealthSnapshot>(`/api/v1/health/list?${FULL_LIST_QS}`);
    return data ?? { summary: emptySummary(), channels: [], ts: 0 };
}

/** Recompute remaining_sec from cooldown_until using local clock. */
export function withLiveCountdowns(snap: HealthSnapshot, nowSec: number): HealthSnapshot {
    const channels = (snap.channels ?? []).map((ch) => ({
        ...ch,
        keys: (ch.keys ?? []).map((k) => {
            let rate_limit = k.rate_limit;
            if (rate_limit?.cooldown_until) {
                const rem = Math.max(0, rate_limit.cooldown_until - nowSec);
                rate_limit = { ...rate_limit, remaining_sec: rem };
            }
            const models = (k.models ?? []).map((m) => {
                if (!m.circuit?.cooldown_until) return m;
                const rem = Math.max(0, m.circuit.cooldown_until - nowSec);
                return {
                    ...m,
                    circuit: { ...m.circuit, remaining_sec: rem },
                };
            });
            return { ...k, rate_limit, models };
        }),
    }));
    return { ...snap, channels };
}

export function formatRemaining(sec: number): string {
    if (sec <= 0) return '0s';
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    if (m <= 0) return `${s}s`;
    return `${m}:${String(s).padStart(2, '0')}`;
}

function channelInGroup(ch: HealthChannel, groupId: number): boolean {
    if (!groupId) return true;
    return (ch.group_ids ?? []).includes(groupId);
}

/** Build summary from a channel list (after group filter; before abnormal-only list trim). */
export function summarizeChannels(channels: HealthChannel[]): HealthSummary {
    const summary = emptySummary();
    summary.channels_total = channels.length;
    for (const ch of channels) {
        switch (ch.status) {
            case 'disabled':
                summary.disabled++;
                summary.channels_abnormal++;
                break;
            case 'circuit_open':
                summary.circuit_open++;
                summary.channels_abnormal++;
                break;
            case 'rate_limited':
                summary.rate_limited++;
                summary.channels_abnormal++;
                break;
            case 'circuit_half_open':
                summary.half_open++;
                summary.channels_abnormal++;
                break;
            case 'degraded':
                summary.degraded++;
                summary.channels_abnormal++;
                break;
            default:
                summary.ok++;
                break;
        }
    }
    return summary;
}

export function filterHealthChannels(
    channels: HealthChannel[],
    params: HealthFilterParams & { search?: string }
): { channels: HealthChannel[]; summary: HealthSummary } {
    const groupId = params.groupId ?? 0;
    const abnormalOnly = params.abnormalOnly !== false;
    const q = (params.search ?? '').trim().toLowerCase();

    const inGroup = channels.filter((ch) => channelInGroup(ch, groupId));
    const summary = summarizeChannels(inGroup);

    let list = inGroup;
    if (abnormalOnly) {
        list = list.filter((ch) => ch.status !== 'ok');
    }
    if (q) {
        list = list.filter((ch) => ch.name.toLowerCase().includes(q));
    }

    return { channels: list, summary };
}

/**
 * Health panel: one full snapshot stream + client-side filters.
 * Switching group / abnormal-only does NOT reconnect SSE.
 */
export function useHealth(params: HealthFilterParams = {}) {
    const { groupId, abnormalOnly = true } = params;
    const [snapshot, setSnapshot] = useState<HealthSnapshot | null>(null);
    const [nowSec, setNowSec] = useState(() => Math.floor(Date.now() / 1000));
    const [isConnected, setIsConnected] = useState(false);
    const [streamError, setStreamError] = useState<Error | null>(null);
    const eventSourceRef = useRef<EventSource | null>(null);
    const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const reconnectAttemptRef = useRef(0);
    const cancelledRef = useRef(false);

    const listQuery = useQuery({
        queryKey: healthListQueryKey,
        queryFn: fetchHealthListFull,
        staleTime: Infinity,
        refetchOnMount: 'always',
        refetchOnWindowFocus: false,
    });

    useEffect(() => {
        if (!listQuery.data) return;
        setSnapshot((prev) => {
            if (!prev) return listQuery.data;
            if ((listQuery.data.ts ?? 0) >= (prev.ts ?? 0)) return listQuery.data;
            return prev;
        });
    }, [listQuery.data]);

    useEffect(() => {
        const id = setInterval(() => setNowSec(Math.floor(Date.now() / 1000)), 1000);
        return () => clearInterval(id);
    }, []);

    const connect = useCallback(async () => {
        if (cancelledRef.current) return;
        try {
            const { token } = await apiClient.get<{ token: string }>('/api/v1/health/stream-token');
            if (cancelledRef.current) return;

            if (eventSourceRef.current) {
                eventSourceRef.current.onerror = null;
                eventSourceRef.current.close();
                eventSourceRef.current = null;
            }

            // Always full snapshot — filters applied client-side so SSE stays open
            const eventSource = new EventSource(
                `${API_BASE_URL}/api/v1/health/stream?token=${token}&${FULL_LIST_QS}`
            );
            eventSourceRef.current = eventSource;

            eventSource.onopen = () => {
                if (eventSourceRef.current !== eventSource) return;
                setIsConnected(true);
                setStreamError(null);
                reconnectAttemptRef.current = 0;
            };

            const onSnapshot = (event: MessageEvent) => {
                if (eventSourceRef.current !== eventSource) return;
                try {
                    const data = JSON.parse(event.data) as HealthSnapshot;
                    setSnapshot(data);
                } catch (e) {
                    logger.error('解析健康快照失败:', e);
                }
            };

            eventSource.addEventListener('snapshot', onSnapshot);

            eventSource.onerror = () => {
                if (eventSourceRef.current !== eventSource) return;

                setIsConnected(false);
                setStreamError(new Error('SSE disconnected'));
                eventSource.onerror = null;
                eventSource.close();
                eventSourceRef.current = null;

                if (cancelledRef.current) return;
                const attempt = reconnectAttemptRef.current + 1;
                reconnectAttemptRef.current = attempt;
                const delay = Math.min(30_000, 1000 * 2 ** Math.min(attempt, 5));
                if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = setTimeout(() => {
                    void connect();
                }, delay);
            };
        } catch (e) {
            if (cancelledRef.current) return;
            setStreamError(e instanceof Error ? e : new Error('stream token failed'));
            logger.error('获取 health stream token 失败:', e);
            const attempt = reconnectAttemptRef.current + 1;
            reconnectAttemptRef.current = attempt;
            const delay = Math.min(30_000, 1000 * 2 ** Math.min(attempt, 5));
            if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
            reconnectTimerRef.current = setTimeout(() => {
                void connect();
            }, delay);
        }
    }, []);

    useEffect(() => {
        cancelledRef.current = false;
        void connect();
        return () => {
            cancelledRef.current = true;
            if (reconnectTimerRef.current) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
            }
            const es = eventSourceRef.current;
            if (es) {
                es.onerror = null;
                es.close();
                eventSourceRef.current = null;
            }
            setIsConnected(false);
        };
    }, [connect]);

    const live = useMemo(() => {
        if (!snapshot) return null;
        return withLiveCountdowns(snapshot, nowSec);
    }, [snapshot, nowSec]);

    const filtered = useMemo(() => {
        return filterHealthChannels(live?.channels ?? [], {
            groupId,
            abnormalOnly,
        });
    }, [live, groupId, abnormalOnly]);

    return {
        snapshot: live,
        summary: filtered.summary,
        channels: filtered.channels,
        isLoading: listQuery.isLoading && !snapshot,
        isConnected,
        streamError,
        refetch: listQuery.refetch,
    };
}
