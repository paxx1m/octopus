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

export type HealthListParams = {
    groupId?: number;
    abnormalOnly?: boolean;
};

function buildListQuery(params: HealthListParams): string {
    const q = new URLSearchParams();
    if (params.groupId && params.groupId > 0) {
        q.set('group_id', String(params.groupId));
    }
    q.set('abnormal_only', params.abnormalOnly === false ? '0' : '1');
    return q.toString();
}

async function fetchHealthList(params: HealthListParams): Promise<HealthSnapshot> {
    const qs = buildListQuery(params);
    const data = await apiClient.get<HealthSnapshot>(`/api/v1/health/list?${qs}`);
    return data ?? { summary: emptySummary(), channels: [], ts: 0 };
}

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

/**
 * Health panel data: initial list + SSE live snapshots + local 1s countdown tick.
 */
export function useHealth(params: HealthListParams = {}) {
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
        queryKey: ['health', 'list', groupId ?? 0, abnormalOnly],
        queryFn: () => fetchHealthList({ groupId, abnormalOnly }),
        staleTime: Infinity,
        refetchOnMount: 'always',
        refetchOnWindowFocus: false,
    });

    // Seed from HTTP list only when filter changes or first load — do not clobber newer SSE snapshots.
    useEffect(() => {
        setSnapshot(null);
    }, [groupId, abnormalOnly]);

    useEffect(() => {
        if (!listQuery.data) return;
        setSnapshot((prev) => {
            if (!prev) return listQuery.data;
            // Prefer fresher SSE/resync data
            if ((listQuery.data.ts ?? 0) >= (prev.ts ?? 0)) return listQuery.data;
            return prev;
        });
    }, [listQuery.data]);

    // local countdown tick
    useEffect(() => {
        const id = setInterval(() => setNowSec(Math.floor(Date.now() / 1000)), 1000);
        return () => clearInterval(id);
    }, []);

    const connect = useCallback(async () => {
        if (cancelledRef.current) return;
        try {
            const { token } = await apiClient.get<{ token: string }>('/api/v1/health/stream-token');
            if (cancelledRef.current) return;

            // Drop any previous connection before opening a new one
            if (eventSourceRef.current) {
                eventSourceRef.current.onerror = null;
                eventSourceRef.current.close();
                eventSourceRef.current = null;
            }

            const qs = buildListQuery({ groupId, abnormalOnly });
            const eventSource = new EventSource(
                `${API_BASE_URL}/api/v1/health/stream?token=${token}&${qs}`
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
            // ignore default onmessage — server always uses event: snapshot | ping

            eventSource.onerror = () => {
                // Stale instance after unmount / reconnect / filter change
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
    }, [groupId, abnormalOnly]);

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

    return {
        snapshot: live,
        summary: live?.summary ?? emptySummary(),
        channels: live?.channels ?? [],
        isLoading: listQuery.isLoading && !snapshot,
        isConnected,
        streamError,
        refetch: listQuery.refetch,
    };
}
