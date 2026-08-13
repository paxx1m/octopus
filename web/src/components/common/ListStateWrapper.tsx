import type { ReactNode } from 'react';
import { Loader2 } from 'lucide-react';

/**
 * 列表三态包装器：统一 loading / error / empty 处理。
 *
 * 补齐 channel/group/model 列表页缺失的加载和错误状态。
 *
 * @example
 * <ListStateWrapper isLoading={isLoading} isError={isError} isEmpty={items.length === 0}>
 *     <VirtualizedGrid items={items} ... />
 * </ListStateWrapper>
 */
export function ListStateWrapper({
    isLoading,
    isError,
    isEmpty,
    error,
    emptyText,
    children,
}: {
    isLoading: boolean;
    isError: boolean;
    isEmpty: boolean;
    error?: unknown;
    emptyText?: string;
    children: ReactNode;
}) {
    if (isLoading) {
        return (
            <div className="flex items-center justify-center py-20">
                <Loader2 className="size-6 animate-spin text-muted-foreground" />
            </div>
        );
    }
    if (isError) {
        return (
            <div className="flex flex-col items-center justify-center py-20 text-destructive">
                <span className="text-sm">
                    {error instanceof Error ? error.message : 'Failed to load data'}
                </span>
            </div>
        );
    }
    if (isEmpty) {
        return (
            <div className="flex items-center justify-center py-20 text-muted-foreground">
                <span className="text-sm">{emptyText ?? 'No data'}</span>
            </div>
        );
    }
    return <>{children}</>;
}
