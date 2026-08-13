import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../client';
import { makeMutation } from '../mutation-helpers';
import { StatsChannel, type StatsMetricsFormatted, formatStatsMetrics } from './stats';
/**
 * 渠道类型枚举
 */
export enum ChannelType {
    OpenAIChat = 'openai/chat_completions',
    OpenAIResponse = 'openai/responses',
    Anthropic = 'anthropic/messages',
    Gemini = 'gemini/contents',
    Volcengine = 'doubao',
    OpenAIEmbedding = 'openai/embeddings',
}

/**
 * 自动分组类型枚举
 */
export enum AutoGroupType {
    None = 0,   // 不自动分组
    Fuzzy = 1,  // 模糊匹配
    Exact = 2,  // 准确匹配
    Regex = 3,  // 正则匹配
}

export type BaseUrl = {
    url: string;
    delay: number;
};

export type CustomHeader = {
    header_key: string;
    header_value: string;
};

export enum KeyMode {
    RoundRobin = 1,
    Random = 2,
    Failover = 3,
    Weighted = 4,
    LeastCost = 5,
}

export type ChannelKey = {
    id: number;
    channel_id: number;
    enabled: boolean;
    channel_key: string;
    status_code: number;
    last_use_time_stamp: number;
    total_cost: number;
    remark: string;
    weight: number;
    rate_limit_cooldown_sec?: number | null;
};

/**
 * 渠道完整数据（与后端 model.Channel 对齐；数组字段在前端保证为 []）
 */
export type Channel = {
    id: number;
    name: string;
    type: ChannelType;
    enabled: boolean;
    base_urls: BaseUrl[];
    keys: ChannelKey[];
    model: string;
    custom_model: string;
    proxy: boolean;
    auto_sync: boolean;
    auto_group: AutoGroupType;
    custom_header: CustomHeader[];
    param_override?: string | null;
    channel_proxy?: string | null;
    match_regex?: string | null;
    key_mode: KeyMode;
    rate_limit_cooldown_sec?: number | null;
    allow_empty_key: boolean;
    stats: StatsChannel;
};

// Internal type: backend may return null for slice fields; normalize to [] in select()
type ChannelServer = Omit<Channel, 'base_urls' | 'custom_header' | 'keys'> & {
    base_urls: BaseUrl[] | null;
    custom_header: CustomHeader[] | null;
    keys: ChannelKey[] | null;
};

/**
 * 创建渠道请求：必填字段 + 可选字段
 */
export type CreateChannelRequest = {
    name: string;
    type: ChannelType;
    enabled?: boolean;
    base_urls: BaseUrl[];
    keys: Array<Pick<ChannelKey, 'enabled' | 'channel_key' | 'remark' | 'weight'> & { rate_limit_cooldown_sec?: number | null }>;
    model: string;
    custom_model?: string;
    proxy?: boolean;
    auto_sync?: boolean;
    auto_group?: AutoGroupType;
    custom_header?: CustomHeader[];
    channel_proxy?: string | null;
    param_override?: string | null;
    match_regex?: string | null;
    key_mode?: KeyMode;
    rate_limit_cooldown_sec?: number | null;
    allow_empty_key?: boolean;
};

/**
 * 更新渠道请求：id + 可选字段 + keys diff
 */
export type UpdateChannelRequest = {
    id: number;
    name?: string;
    type?: ChannelType;
    enabled?: boolean;
    base_urls?: BaseUrl[];
    model?: string;
    custom_model?: string;
    proxy?: boolean;
    auto_sync?: boolean;
    auto_group?: AutoGroupType;
    custom_header?: CustomHeader[];
    channel_proxy?: string | null;
    param_override?: string | null;
    match_regex?: string | null;
    key_mode?: KeyMode;
    rate_limit_cooldown_sec?: number | null;
    clear_rate_limit_cooldown?: boolean;
    allow_empty_key?: boolean;
    // keys diff
    keys_to_add?: Array<Pick<ChannelKey, 'enabled' | 'channel_key' | 'remark' | 'weight'> & { rate_limit_cooldown_sec?: number | null }>;
    keys_to_update?: Array<{
        id: number;
        enabled?: boolean;
        channel_key?: string;
        remark?: string;
        weight?: number;
        rate_limit_cooldown_sec?: number | null;
        clear_rate_limit_cooldown?: boolean;
    }>;
    keys_to_delete?: number[];
};

export type FetchModelRequest = {
    type: ChannelType;
    base_urls: BaseUrl[];
    keys: Array<Pick<ChannelKey, 'enabled' | 'channel_key'>>;
    proxy?: boolean;
    channel_proxy?: string | null;
    match_regex?: string | null;
    custom_header?: CustomHeader[];
};

/**
 * 获取渠道列表 Hook
 * 
 * @example
 * const { data: channels, isLoading, error } = useChannelList();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * channels?.forEach(channel => console.log(channel.raw.name));
 */
export function useChannelList() {
    return useQuery({
        queryKey: ['channels', 'list'],
        queryFn: async () => {
            return apiClient.get<ChannelServer[]>('/api/v1/channel/list');
        },
        select: (data) => data.map((item) => ({
            raw: ({
                ...item,
                base_urls: item.base_urls ?? [],
                custom_header: item.custom_header ?? [],
                keys: (item.keys ?? []).map((k) => ({
                    ...k,
                    weight: k.weight || 1,
                })),
                key_mode: item.key_mode || KeyMode.LeastCost,
                allow_empty_key: item.allow_empty_key ?? false,
            }) satisfies Channel,
            formatted: formatStatsMetrics(item.stats ?? {
                input_token: 0,
                output_token: 0,
                input_cost: 0,
                output_cost: 0,
                wait_time: 0,
                output_time: 0,
                request_success: 0,
                request_failed: 0,
            }),
        })) as Array<{ raw: Channel; formatted: StatsMetricsFormatted }>,
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

/**
 * 创建渠道 Hook
 * 
 * @example
 * const createChannel = useCreateChannel();
 * 
 * createChannel.mutate({
 *   name: 'OpenAI',
 *   type: ChannelType.OpenAIChat,
 *   base_urls: [{ url: 'https://api.openai.com', delay: 0 }],
 *   keys: [{ enabled: true, channel_key: 'sk-xxx' }],
 *   model: 'gpt-4',
 * });
 */
export function useCreateChannel() {
    return makeMutation<CreateChannelRequest, ChannelServer>({
        name: '渠道创建',
        mutationFn: (data) => apiClient.post<ChannelServer>('/api/v1/channel/create', data),
        invalidate: [['channels', 'list'], ['models', 'list'], ['models', 'channel']],
    });
}

/**
 * 更新渠道 Hook
 * 
 * @example
 * const updateChannel = useUpdateChannel();
 * 
 * updateChannel.mutate({
 *   id: 1,
 *   name: 'OpenAI Updated',
 *   type: ChannelType.OpenAIChat,
 *   enabled: true,
 *   base_urls: [{ url: 'https://api.openai.com', delay: 0 }],
 *   keys_to_add: [{ enabled: true, channel_key: 'sk-xxx' }],
 *   model: 'gpt-4-turbo',
 *   proxy: false,
 * });
 */
export function useUpdateChannel() {
    return makeMutation<UpdateChannelRequest, ChannelServer>({
        name: '渠道更新',
        mutationFn: (data) => apiClient.post<ChannelServer>('/api/v1/channel/update', data),
        invalidate: [['channels', 'list'], ['models', 'channel']],
    });
}

/**
 * 删除渠道 Hook
 * 
 * @example
 * const deleteChannel = useDeleteChannel();
 * 
 * deleteChannel.mutate(1); // 删除 ID 为 1 的渠道
 */
export function useDeleteChannel() {
    return makeMutation<number, null>({
        name: '渠道删除',
        mutationFn: (id) => apiClient.delete<null>(`/api/v1/channel/delete/${id}`),
        invalidate: [['channels', 'list'], ['models', 'channel']],
    });
}

/**
 * 启用/禁用渠道 Hook
 * 
 * @example
 * const enableChannel = useEnableChannel();
 * 
 * enableChannel.mutate({ id: 1, enabled: true }); // 启用 ID 为 1 的渠道
 * enableChannel.mutate({ id: 1, enabled: false }); // 禁用 ID 为 1 的渠道
 */
export function useEnableChannel() {
    return makeMutation<{ id: number; enabled: boolean }, null>({
        name: '渠道状态更新',
        mutationFn: (data) => apiClient.post<null>('/api/v1/channel/enable', data),
        invalidate: [['channels', 'list']],
    });
}

/**
 * 获取渠道模型列表 Hook
 * 
 * @example
 * const fetchModel = useFetchModel();
 * 
 * fetchModel.mutate({
 *   type: ChannelType.OpenAIChat,
 *   base_urls: [{ url: 'https://api.openai.com', delay: 0 }],
 *   keys: [{ enabled: true, channel_key: 'sk-xxx' }],
 *   proxy: false,
 * });
 * 
 * // 在 onSuccess 中获取模型列表
 * fetchModel.data // ['gpt-4', 'gpt-3.5-turbo', ...]
 */
export function useFetchModel() {
    return makeMutation<FetchModelRequest, string[]>({
        name: '模型列表获取',
        mutationFn: (data) => apiClient.post<string[]>('/api/v1/channel/fetch-model', data),
    });
}

/**
 * 获取渠道最后同步时间 Hook
 * 
 * @example
 * const lastSyncTime = useLastSyncTime();
 * 
 * if (lastSyncTime) {
 *   console.log('最后同步时间:', new Date(lastSyncTime).toLocaleString());
 * }
 */
export function useLastSyncTime() {
    return useQuery({
        queryKey: ['channels', 'last-sync-time'],
        queryFn: async () => {
            return apiClient.get<string>('/api/v1/channel/last-sync-time');
        },
        refetchInterval: 30000,
    });
}

