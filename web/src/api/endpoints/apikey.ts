import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../client';
import { useAppMutation } from '../mutation-helpers';
import { useAuthStore } from './user';
import { StatsAPIKey, StatsAPIKeyFormatted, formatStatsMetrics } from './stats';

/**
 * API Key 数据
 */
export interface APIKey {
    id: number;
    name: string;
    api_key: string;
    enabled: boolean;
    expire_at?: number; // Unix 时间戳（秒），不传表示永不过期
    max_cost?: number; // 不传表示无限制
    supported_models?: string; // 不传表示支持所有模型
}

/**
 * API Key Stats 响应（包含 stats 和 info）
 */
export interface APIKeyStatsResponse {
    stats: StatsAPIKey;
    info: APIKey;
}

export interface APIKeyStatsResponseFormatted {
    stats: StatsAPIKeyFormatted;
    info: APIKey;
}

/**
 * API Key 登录 Hook（仅校验 key 是否有效）
 */
export function useAPIKeyLogin() {
    const { setAPIKeyAuth, logout } = useAuthStore();

    return useAppMutation<string, string>({
        name: 'API Key 登录',
        mutationFn: async (apiKey) => {
            // 先设置以便 apiClient 发送请求时带上 token
            setAPIKeyAuth(apiKey);
            await apiClient.get<null>('/api/v1/apikey/login');
            return apiKey;
        },
        onError: () => {
            logout();
        },
    });
}

/**
 * 获取当前 API Key 的详细统计数据 Hook（仅 API Key 登录用户使用）
 */
export function useAPIKeyDashboardStats() {
    const { isAPIKeyAuth, isAuthenticated } = useAuthStore();

    return useQuery({
        queryKey: ['apikey', 'dashboard', 'stats'],
        queryFn: () => apiClient.get<APIKeyStatsResponse>('/api/v1/apikey/stats'),
        select: (data): APIKeyStatsResponseFormatted => ({
            stats: {
                ...formatStatsMetrics(data.stats),
                api_key_id: data.stats.api_key_id,
            },
            info: data.info,
        }),
        enabled: isAPIKeyAuth && isAuthenticated,
        refetchInterval: 30000,
    });
}

/**
 * 创建/编辑 API Key 的表单数据（不含 id 与明文 key）
 */
export type APIKeyFormData = Omit<APIKey, 'id' | 'api_key'>;

/**
 * 创建 API Key 请求
 */
export type CreateAPIKeyRequest = APIKeyFormData & { enabled?: boolean };

/**
 * 更新 API Key 请求
 */
export type UpdateAPIKeyRequest = Pick<APIKey, 'id'> & CreateAPIKeyRequest;

/**
 * 获取 API Key 列表 Hook
 * 
 * @example
 * const { data: apiKeys, isLoading, error } = useAPIKeyList();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * apiKeys?.forEach(key => console.log(key.name));
 */
export function useAPIKeyList() {
    return useQuery({
        queryKey: ['apikeys', 'list'],
        queryFn: async () => {
            return apiClient.get<APIKey[]>('/api/v1/apikey/list');
        },
        refetchInterval: 30000,
    });
}

/**
 * 创建 API Key Hook
 * 
 * @example
 * const createAPIKey = useCreateAPIKey();
 * 
 * createAPIKey.mutate({
 *   name: 'My API Key',
 * });
 */
export function useCreateAPIKey() {
    return useAppMutation<CreateAPIKeyRequest, APIKey>({
        name: 'API Key 创建',
        mutationFn: (data) => apiClient.post<APIKey>('/api/v1/apikey/create', data),
        invalidate: [['apikeys', 'list']],
    });
}

/**
 * 更新 API Key Hook
 * 
 * @example
 * const updateAPIKey = useUpdateAPIKey();
 * 
 * updateAPIKey.mutate({
 *   id: 1,
 *   name: 'Updated API Key',
 *   enabled: false,
 * });
 */
export function useUpdateAPIKey() {
    return useAppMutation<UpdateAPIKeyRequest, APIKey>({
        name: 'API Key 更新',
        mutationFn: (data) => apiClient.post<APIKey>('/api/v1/apikey/update', data),
        invalidate: [['apikeys', 'list']],
    });
}

/**
 * 删除 API Key Hook
 * 
 * @example
 * const deleteAPIKey = useDeleteAPIKey();
 * 
 * deleteAPIKey.mutate(1); // 删除 ID 为 1 的 API Key
 */
export function useDeleteAPIKey() {
    return useAppMutation<number, null>({
        name: 'API Key 删除',
        mutationFn: (id) => apiClient.delete<null>(`/api/v1/apikey/delete/${id}`),
        invalidate: [['apikeys', 'list']],
    });
}
