import { useMutation, useQueryClient, type UseMutationOptions, type UseMutationResult } from '@tanstack/react-query';
import { logger } from '@/lib/logger';
import { toast } from '@/components/common/Toast';

type QueryKey = readonly unknown[];

export interface MakeMutationOptions<TVars, TData, TError extends Error = Error> {
    mutationFn: (vars: TVars) => Promise<TData>;
    /** 操作名称，用于日志（如 "渠道创建"）；自动拼成 "渠道创建成功" / "渠道创建失败" */
    name: string;
    /** 成功后需要 invalidate 的 queryKey 列表 */
    invalidate?: QueryKey[];
    /** 额外的 onSuccess 回调（在 invalidate 之后执行） */
    onSuccess?: (data: TData, vars: TVars) => void;
    /** 额外的 onError 回调 */
    onError?: (error: TError, vars: TVars) => void;
}

/**
 * 创建带统一日志 + invalidate 模板的 mutation hook。
 *
 * 消除每个 endpoint 中重复的：
 * - onSuccess: logger.log('XX成功') + invalidateQueries
 * - onError: logger.error('XX失败')
 *
 * @example
 * export function useCreateChannel() {
 *     return makeMutation({
 *         name: '渠道创建',
 *         mutationFn: (data: CreateChannelRequest) => apiClient.post('/api/v1/channel/create', data),
 *         invalidate: [['channels', 'list'], ['models', 'list']],
 *     });
 * }
 */
// eslint-disable-next-line react-hooks/rules-of-hooks -- makeMutation 是工厂 hook，语义等同 useMutation 的薄包装
export function makeMutation<TVars, TData = unknown, TError extends Error = Error>(opts: MakeMutationOptions<TVars, TData, TError>) {
    const queryClient = useQueryClient();

    const mutationOptions: UseMutationOptions<TData, TError, TVars> = {
        mutationFn: opts.mutationFn,
        onSuccess: (data, vars) => {
            logger.log(`${opts.name}成功:`, data);
            opts.invalidate?.forEach((key) => {
                queryClient.invalidateQueries({ queryKey: key });
            });
            opts.onSuccess?.(data, vars);
        },
        onError: (error, vars) => {
            logger.error(`${opts.name}失败:`, error);
            opts.onError?.(error, vars);
        },
    };

    return useMutation(mutationOptions);
}

export interface UseToastMutationOptions<TVars, TData, TError extends Error = Error> {
    mutationFn: (vars: TVars) => Promise<TData>;
    /** 操作名称，用于日志 */
    name: string;
    /** 成功 toast 文案 */
    successMsg: string;
    /** 失败 toast 文案（默认使用 error.message） */
    errorMsg?: string;
    /** 成功后需要 invalidate 的 queryKey 列表 */
    invalidate?: QueryKey[];
    /** 额外的 onSuccess 回调（在 toast + invalidate 之后执行） */
    onSuccess?: (data: TData, vars: TVars) => void;
    /** 额外的 onError 回调 */
    onError?: (error: TError, vars: TVars) => void;
}

/**
 * 创建带 toast 反馈的 mutation hook。
 *
 * 在 makeMutation 基础上叠加 toast.success / toast.error，
 * 用于需要即时用户反馈的写操作（如设置保存、密码修改）。
 *
 * @example
 * const updatePrice = useToastMutation({
 *     name: '模型价格更新',
 *     mutationFn: () => apiClient.post('/api/v1/model/update-price', {}),
 *     successMsg: t('llmPrice.updateSuccess'),
 *     errorMsg: t('llmPrice.updateFailed'),
 *     invalidate: [['models', 'last-update-time']],
 * });
 */
export function useToastMutation<TVars, TData = unknown, TError extends Error = Error>(opts: UseToastMutationOptions<TVars, TData, TError>): UseMutationResult<TData, TError, TVars> {
    const queryClient = useQueryClient();

    return useMutation<TData, TError, TVars>({
        mutationFn: opts.mutationFn,
        onSuccess: (data, vars) => {
            logger.log(`${opts.name}成功:`, data);
            toast.success(opts.successMsg);
            opts.invalidate?.forEach((key) => {
                queryClient.invalidateQueries({ queryKey: key });
            });
            opts.onSuccess?.(data, vars);
        },
        onError: (error, vars) => {
            logger.error(`${opts.name}失败:`, error);
            toast.error(opts.errorMsg ?? error.message);
            opts.onError?.(error, vars);
        },
    });
}
