import { useCallback, useEffect, useRef, useState } from 'react';
import { API_BASE_URL, apiClient } from '../client';

export interface SSEStreamOptions {
    /** 获取 stream-token 的 GET 接口（需要鉴权） */
    tokenUrl: string;
    /** 建流路径（拼接 token 参数） */
    streamPath: string;
    /** 事件处理器，key 为 SSE 事件名（'message' 对应默认事件） */
    events?: Record<string, (event: MessageEvent) => void>;
    /** 断线后是否指数退避重连，默认 true */
    reconnect?: boolean;
    maxReconnectDelay?: number;
}

/**
 * 统一 SSE 订阅：取 token → 建 EventSource → 断线退避重连 → 卸载清理。
 * 消除 log / health 两套几乎相同的 stream-token + EventSource + 重连实现。
 */
export function useSSEStream({
    tokenUrl,
    streamPath,
    events,
    reconnect = true,
    maxReconnectDelay = 30_000,
}: SSEStreamOptions) {
    const [isConnected, setIsConnected] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    const eventSourceRef = useRef<EventSource | null>(null);
    const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const reconnectAttemptRef = useRef(0);
    const cancelledRef = useRef(false);
    // 事件处理器放入 ref，避免每次渲染重建连接
    const eventsRef = useRef(events);
    eventsRef.current = events;

    const scheduleReconnect = useCallback(
        (fn: () => void) => {
            if (cancelledRef.current) return;
            const attempt = reconnectAttemptRef.current + 1;
            reconnectAttemptRef.current = attempt;
            const delay = Math.min(maxReconnectDelay, 1000 * 2 ** Math.min(attempt, 5));
            if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
            reconnectTimerRef.current = setTimeout(fn, delay);
        },
        [maxReconnectDelay]
    );

    const connect = useCallback(async () => {
        if (cancelledRef.current) return;
        try {
            const { token } = await apiClient.get<{ token: string }>(tokenUrl);
            if (cancelledRef.current) return;

            if (eventSourceRef.current) {
                eventSourceRef.current.onerror = null;
                eventSourceRef.current.close();
                eventSourceRef.current = null;
            }

            const sep = streamPath.includes('?') ? '&' : '?';
            const eventSource = new EventSource(`${API_BASE_URL}${streamPath}${sep}token=${token}`);
            eventSourceRef.current = eventSource;

            eventSource.onopen = () => {
                if (eventSourceRef.current !== eventSource) return;
                setIsConnected(true);
                setError(null);
                reconnectAttemptRef.current = 0;
            };

            const onEvent = (event: MessageEvent) => {
                if (eventSourceRef.current !== eventSource) return;
                eventsRef.current?.[event.type]?.(event);
            };
            for (const name of Object.keys(eventsRef.current ?? {})) {
                if (name === 'message') {
                    eventSource.onmessage = onEvent;
                } else {
                    eventSource.addEventListener(name, onEvent);
                }
            }

            eventSource.onerror = () => {
                if (eventSourceRef.current !== eventSource) return;
                setIsConnected(false);
                setError(new Error('SSE 连接断开'));
                eventSource.onerror = null;
                eventSource.close();
                eventSourceRef.current = null;
                if (reconnect) {
                    scheduleReconnect(() => {
                        void connect();
                    });
                }
            };
        } catch (e) {
            if (cancelledRef.current) return;
            setError(e instanceof Error ? e : new Error('获取 stream token 失败'));
            if (reconnect) {
                scheduleReconnect(() => {
                    void connect();
                });
            }
        }
    }, [tokenUrl, streamPath, reconnect, scheduleReconnect]);

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

    return { isConnected, error };
}
