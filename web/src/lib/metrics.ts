/**
 * Shared performance metrics derived from existing cumulative stats / per-request logs.
 * No extra API or DB fields — pure client-side math.
 */

export function requestCount(success: number, failed: number): number {
    const s = Number.isFinite(success) ? Math.max(0, success) : 0;
    const f = Number.isFinite(failed) ? Math.max(0, failed) : 0;
    return s + f;
}

/** Success rate in percent [0, 100]. Returns 0 when no requests. */
export function successRatePercent(success: number, failed: number): number {
    const total = requestCount(success, failed);
    if (total <= 0) return 0;
    const s = Number.isFinite(success) ? Math.max(0, success) : 0;
    return (s / total) * 100;
}

/** Average latency in ms from cumulative wait_time sum and request count. */
export function avgLatencyMs(waitTimeSumMs: number, success: number, failed: number): number {
    const total = requestCount(success, failed);
    if (total <= 0) return 0;
    const sum = Number.isFinite(waitTimeSumMs) ? Math.max(0, waitTimeSumMs) : 0;
    return sum / total;
}

/**
 * Tokens per second over a duration window.
 * @param tokens token count
 * @param durationMs elapsed milliseconds
 */
export function tokensPerSecond(tokens: number, durationMs: number): number {
    if (!Number.isFinite(tokens) || !Number.isFinite(durationMs)) return 0;
    if (tokens <= 0 || durationMs <= 0) return 0;
    return tokens / (durationMs / 1000);
}

/**
 * Output tokens/s after first token (excludes TTFT).
 * Non-stream (firstTokenMs <= 0): falls back to output / totalDuration.
 */
export function outputTokensPerSecond(
    outputTokens: number,
    totalDurationMs: number,
    firstTokenMs = 0,
): number {
    if (!Number.isFinite(outputTokens) || outputTokens <= 0) return 0;
    if (!Number.isFinite(totalDurationMs) || totalDurationMs <= 0) return 0;
    const ttft = Number.isFinite(firstTokenMs) && firstTokenMs > 0 ? firstTokenMs : 0;
    const generationMs = ttft > 0 ? totalDurationMs - ttft : totalDurationMs;
    if (generationMs <= 0) return 0;
    return outputTokens / (generationMs / 1000);
}

export function formatSuccessRate(rate: number): string {
    if (!Number.isFinite(rate) || rate <= 0) return '0%';
    if (rate >= 99.95) return '100%';
    if (rate >= 10) return `${rate.toFixed(1)}%`;
    return `${rate.toFixed(2)}%`;
}

export function formatTokensPerSec(tps: number): string {
    if (!Number.isFinite(tps) || tps <= 0) return '—';
    if (tps >= 1000) return `${(tps / 1000).toFixed(2)}k t/s`;
    if (tps >= 100) return `${tps.toFixed(0)} t/s`;
    if (tps >= 10) return `${tps.toFixed(1)} t/s`;
    return `${tps.toFixed(2)} t/s`;
}

export function formatAvgLatency(ms: number): string {
    if (!Number.isFinite(ms) || ms <= 0) return '—';
    if (ms < 1000) return `${Math.round(ms)}ms`;
    if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`;
    return `${(ms / 60_000).toFixed(2)}m`;
}
