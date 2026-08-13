import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../client';
import { useAppMutation } from '../mutation-helpers';

/**
 * 后端 /api/v1/update 返回的最新发布信息
 */
export interface LatestInfo {
    tag_name: string;
    published_at: string;
    body: string;
    message: string;
}

/**
 * 获取最新发布信息 Hook
 * 
 * @example
 * const { data: latestInfo, isLoading, error } = useLatestInfo();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * console.log('Latest tag:', latestInfo?.tag_name);
 */
export function useLatestInfo() {
    return useQuery({
        queryKey: ['update', 'latest'],
        queryFn: async () => {
            return apiClient.get<LatestInfo>('/api/v1/update');
        },
        refetchInterval: 3600000, // 1 小时
        refetchOnMount: 'always',
    });
}

/**
 * 获取后端当前版本 Hook
 *
 * 后端: GET /api/v1/update/now-version -> string
 */
export function useNowVersion() {
    return useQuery({
        queryKey: ['update', 'now-version'],
        queryFn: async () => {
            return apiClient.get<string>('/api/v1/update/now-version');
        },
        refetchInterval: 3600000, // 1 小时
        refetchOnMount: 'always',
    });
}

/**
 * 执行更新 Hook
 * 
 * @example
 * const updateCore = useUpdateCore();
 * 
 * updateCore.mutate(undefined, {
 *   onSuccess: () => {
 *     console.log('Update started successfully');
 *   },
 * });
 */
export function useUpdateCore() {
    return useAppMutation<void, string>({
        name: '更新',
        mutationFn: () => apiClient.post<string>('/api/v1/update'),
        invalidate: [['update', 'latest'], ['update', 'now-version']],
    });
}

