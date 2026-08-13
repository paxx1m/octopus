import { useMutation, useQuery } from '@tanstack/react-query';
import { apiClient, type BlobDownload } from '../client';
import { logger } from '@/lib/logger';
import { makeMutation } from '../mutation-helpers';

/**
 * Setting 数据
 */
export interface Setting {
    key: string;
    value: string;
}

export const SettingKey = {
    ProxyURL: 'proxy_url',
    StatsSaveInterval: 'stats_save_interval',
    ModelInfoUpdateInterval: 'model_info_update_interval',
    SyncLLMInterval: 'sync_llm_interval',
    RelayLogKeepEnabled: 'relay_log_keep_enabled',
    RelayLogKeepPeriod: 'relay_log_keep_period',
    CORSAllowOrigins: 'cors_allow_origins',
    CircuitBreakerThreshold: 'circuit_breaker_threshold',
    CircuitBreakerCooldown: 'circuit_breaker_cooldown',
    CircuitBreakerMaxCooldown: 'circuit_breaker_max_cooldown',
    ChannelKeyRateLimitCooldown: 'channel_key_rate_limit_cooldown',
} as const;

/**
 * 获取 Setting 列表 Hook
 * 
 * @example
 * const { data: settings, isLoading, error } = useSettingList();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * settings?.forEach(setting => console.log(setting.key, setting.value));
 */
export function useSettingList() {
    return useQuery({
        queryKey: ['settings', 'list'],
        queryFn: async () => {
            return apiClient.get<Setting[]>('/api/v1/setting/list');
        },
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

/**
 * 设置 Setting Hook
 * 
 * @example
 * const setSetting = useSetSetting();
 * 
 * setSetting.mutate({
 *   key: 'theme',
 *   value: 'dark',
 * });
 */
export function useSetSetting() {
    return makeMutation<Setting, Setting>({
        name: 'Setting 设置',
        mutationFn: (data) => apiClient.post<Setting>('/api/v1/setting/set', data),
        invalidate: [['settings', 'list']],
    });
}

/**
 * 数据库导入/导出
 */
export interface DBImportResult {
    rows_affected: Record<string, number>;
}

export interface DBExportOptions {
    include_logs?: boolean;
    include_stats?: boolean;
}

function exportFallbackFilename() {
    const d = new Date();
    const pad = (n: number) => String(n).padStart(2, '0');
    const ts = `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`;
    return `octopus-export-${ts}.json`;
}

async function downloadBlob(blob: Blob, filename: string) {
    const url = URL.createObjectURL(blob);
    try {
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
    } finally {
        URL.revokeObjectURL(url);
    }
}

/**
 * 导出数据库（下载 JSON 文件）
 */
export function useExportDB() {
    return useMutation({
        mutationFn: async (options: DBExportOptions = {}) => {
            // 大库导出可能超过默认 120s 超时，放宽到 10 分钟
            const res: BlobDownload = await apiClient.getBlob('/api/v1/setting/export', {
                include_logs: String(!!options.include_logs),
                include_stats: String(!!options.include_stats),
            }, 600_000);
            const filename = res.filename || exportFallbackFilename();
            await downloadBlob(res.blob, filename);
            return { filename };
        },
        onError: (error) => {
            logger.error('导出数据库失败:', error);
        },
    });
}

/**
 * 导入数据库（上传 JSON 文件，增量导入）
 */
export function useImportDB() {
    return useMutation({
        mutationFn: async (file: File) => {
            const form = new FormData();
            form.append('file', file);
            // 大文件导入可能超过默认 120s 超时，放宽到 10 分钟
            return apiClient.postForm<DBImportResult>('/api/v1/setting/import', form, 600_000);
        },
        onError: (error) => {
            logger.error('导入数据库失败:', error);
        },
    });
}

