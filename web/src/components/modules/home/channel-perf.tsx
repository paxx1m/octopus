import { useMemo, useState } from 'react';
import { useTranslations } from 'use-intl';
import { Activity, ArrowDownUp, CheckCircle2, Clock, Gauge, TrendingUp } from 'lucide-react';
import { useChannelList } from '@/api/endpoints/channel';
import { cn } from '@/lib/utils';

type SortKey = 'success_rate' | 'avg_latency' | 'tokens_per_sec' | 'request_count';

export function ChannelPerf() {
    const t = useTranslations('home.channelPerf');
    const { data: channelData } = useChannelList();
    const [sortKey, setSortKey] = useState<SortKey>('request_count');

    const rows = useMemo(() => {
        if (!channelData?.length) return [];
        const list = channelData.map((ch) => ({
            id: ch.raw.id,
            name: ch.raw.name,
            enabled: ch.raw.enabled,
            request_count: ch.formatted.request_count.raw,
            success_rate: ch.formatted.success_rate,
            success_rate_label: ch.formatted.success_rate_label,
            avg_latency_ms: ch.formatted.avg_latency_ms,
            avg_latency_label: ch.formatted.avg_latency_label,
            tokens_per_sec: ch.formatted.tokens_per_sec,
            tokens_per_sec_label: ch.formatted.tokens_per_sec_label,
            request_count_label: `${ch.formatted.request_count.formatted.value}${ch.formatted.request_count.formatted.unit}`,
        }));

        const sorted = [...list];
        sorted.sort((a, b) => {
            switch (sortKey) {
                case 'success_rate':
                    return b.success_rate - a.success_rate || b.request_count - a.request_count;
                case 'avg_latency':
                    // lower latency first; channels with no traffic sink
                    if (a.request_count === 0 && b.request_count === 0) return 0;
                    if (a.request_count === 0) return 1;
                    if (b.request_count === 0) return -1;
                    return a.avg_latency_ms - b.avg_latency_ms;
                case 'tokens_per_sec':
                    return b.tokens_per_sec - a.tokens_per_sec || b.request_count - a.request_count;
                case 'request_count':
                default:
                    return b.request_count - a.request_count;
            }
        });
        return sorted;
    }, [channelData, sortKey]);

    const sortOptions: { key: SortKey; label: string; icon: typeof Activity }[] = [
        { key: 'request_count', label: t('sort.requests'), icon: Activity },
        { key: 'success_rate', label: t('sort.successRate'), icon: CheckCircle2 },
        { key: 'avg_latency', label: t('sort.avgLatency'), icon: Clock },
        { key: 'tokens_per_sec', label: t('sort.tokensPerSec'), icon: Gauge },
    ];

    return (
        <section className="rounded-3xl border border-border bg-card p-5 text-card-foreground">
            <header className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                    <TrendingUp className="size-5 text-primary" />
                    <h2 className="text-lg font-bold">{t('title')}</h2>
                </div>
                <div className="flex flex-wrap gap-1">
                    {sortOptions.map(({ key, label, icon: Icon }) => (
                        <button
                            key={key}
                            type="button"
                            onClick={() => setSortKey(key)}
                            className={cn(
                                'inline-flex h-8 items-center gap-1 rounded-xl px-2.5 text-xs font-medium transition-colors',
                                sortKey === key
                                    ? 'bg-primary text-primary-foreground'
                                    : 'bg-muted/40 text-muted-foreground hover:bg-muted hover:text-foreground',
                            )}
                        >
                            <Icon className="size-3.5" />
                            {label}
                        </button>
                    ))}
                </div>
            </header>

            {rows.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-10 text-muted-foreground">
                    <ArrowDownUp className="mb-3 size-10 opacity-30" />
                    <p className="text-sm">{t('noData')}</p>
                </div>
            ) : (
                <div className="overflow-x-auto overscroll-contain">
                    <table className="w-full min-w-[36rem] border-collapse text-sm">
                        <thead>
                            <tr className="border-b border-border/60 text-left text-xs text-muted-foreground">
                                <th className="px-2 py-2 font-medium">#</th>
                                <th className="px-2 py-2 font-medium">{t('columns.channel')}</th>
                                <th className="px-2 py-2 text-right font-medium">{t('columns.requests')}</th>
                                <th className="px-2 py-2 text-right font-medium">{t('columns.successRate')}</th>
                                <th className="px-2 py-2 text-right font-medium">{t('columns.avgLatency')}</th>
                                <th className="px-2 py-2 text-right font-medium">{t('columns.tokensPerSec')}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((row, index) => (
                                <tr
                                    key={row.id}
                                    className="border-b border-border/40 last:border-0 hover:bg-muted/30"
                                >
                                    <td className="px-2 py-2.5 tabular-nums text-muted-foreground">
                                        {index + 1}
                                    </td>
                                    <td className="max-w-[12rem] px-2 py-2.5">
                                        <div className="flex min-w-0 items-center gap-2">
                                            <span
                                                className={cn(
                                                    'size-1.5 shrink-0 rounded-full',
                                                    row.enabled ? 'bg-emerald-500' : 'bg-muted-foreground/40',
                                                )}
                                                title={row.enabled ? t('enabled') : t('disabled')}
                                            />
                                            <span className="truncate font-medium" title={row.name}>
                                                {row.name}
                                            </span>
                                        </div>
                                    </td>
                                    <td className="px-2 py-2.5 text-right tabular-nums">
                                        {row.request_count_label}
                                    </td>
                                    <td
                                        className={cn(
                                            'px-2 py-2.5 text-right tabular-nums font-medium',
                                            row.request_count === 0
                                                ? 'text-muted-foreground'
                                                : row.success_rate >= 95
                                                  ? 'text-emerald-600 dark:text-emerald-400'
                                                  : row.success_rate >= 80
                                                    ? 'text-amber-600 dark:text-amber-400'
                                                    : 'text-destructive',
                                        )}
                                    >
                                        {row.request_count === 0 ? '—' : row.success_rate_label}
                                    </td>
                                    <td className="px-2 py-2.5 text-right tabular-nums text-muted-foreground">
                                        {row.request_count === 0 ? '—' : row.avg_latency_label}
                                    </td>
                                    <td className="px-2 py-2.5 text-right tabular-nums">
                                        {row.request_count === 0 || row.tokens_per_sec <= 0
                                            ? '—'
                                            : row.tokens_per_sec_label}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}

            <p className="mt-3 text-[11px] leading-relaxed text-muted-foreground/80">
                {t('hint')}
            </p>
        </section>
    );
}
