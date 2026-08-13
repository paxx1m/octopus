import type { ReactNode } from 'react';

/**
 * 统计指标卡片：统一 dl/dt/dd 结构 + 渐变背景 + 图标。
 *
 * 替代 channel/CardContent、setting/APIKey、apikey-dashboard 中重复的统计小卡片。
 *
 * @example
 * <StatCard
 *     icon={<Activity className="size-4 text-chart-1" />}
 *     label={t('metrics.totalRequests')}
 *     value={stats.request_count.formatted.value}
 *     unit={stats.request_count.formatted.unit}
 *     gradient="from-chart-1/10 to-chart-1/5"
 *     valueClassName="text-chart-1"
 * />
 */
export function StatCard({
    icon,
    label,
    value,
    unit,
    gradient = 'from-muted/10 to-muted/5',
    valueClassName = '',
    className = '',
}: {
    icon: ReactNode;
    label: string;
    value: ReactNode;
    unit?: string;
    /** 背景渐变，如 "from-chart-1/10 to-chart-1/5" */
    gradient?: string;
    /** 数值颜色，如 "text-chart-1" */
    valueClassName?: string;
    className?: string;
}) {
    return (
        <div className={`rounded-2xl border bg-linear-to-br ${gradient} p-3 sm:p-4 ${className}`}>
            <dt className="flex items-center gap-2 mb-2 text-xs font-medium text-muted-foreground">
                {icon}
                {label}
            </dt>
            <dd className={`text-xl sm:text-2xl font-bold ${valueClassName}`}>
                {value}
                {unit && <span className="text-xs font-normal ml-1 text-muted-foreground">{unit}</span>}
            </dd>
        </div>
    );
}

/**
 * 紧凑型统计卡片（用于 setting 页等小尺寸场景）。
 */
export function StatCardCompact({
    icon,
    label,
    value,
    unit,
    className = '',
}: {
    icon: ReactNode;
    label: string;
    value: ReactNode;
    unit?: string;
    className?: string;
}) {
    return (
        <div className={`rounded-lg bg-muted/40 p-3 ${className}`}>
            <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
                {icon}
                {label}
            </dt>
            <dd className="text-lg font-semibold mt-1">
                {value}
                {unit && <span className="text-xs font-normal ml-1 text-muted-foreground">{unit}</span>}
            </dd>
        </div>
    );
}
