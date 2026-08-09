import { useMemo, useState } from 'react';
import { useHealth } from '@/api/endpoints/health';
import { useGroupList } from '@/api/endpoints/group';
import { ChannelRow } from './ChannelRow';
import { PageWrapper } from '@/components/common/PageWrapper';
import { useTranslations } from 'use-intl';
import { Loader2, Activity } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Input } from '@/components/ui/input';

export function Health() {
    const t = useTranslations('health');
    const [groupId, setGroupId] = useState<number>(0);
    const [abnormalOnly, setAbnormalOnly] = useState(true);
    const [search, setSearch] = useState('');

    const { channels, summary, isLoading, isConnected, streamError } = useHealth({
        groupId: groupId || undefined,
        abnormalOnly,
    });
    const { data: groups } = useGroupList();

    const filtered = useMemo(() => {
        const q = search.trim().toLowerCase();
        if (!q) return channels;
        return channels.filter((c) => c.name.toLowerCase().includes(q));
    }, [channels, search]);

    return (
        <PageWrapper className="h-full min-h-0 space-y-4 overflow-y-auto overscroll-contain rounded-t-3xl pb-24 scrollbar-none md:pb-4">
            {/* summary + connection */}
            <div className="rounded-3xl border border-border bg-card p-4 space-y-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                        <Activity className="size-4 text-primary" />
                        <h2 className="text-sm font-semibold">{t('title')}</h2>
                        <span
                            className={cn(
                                'inline-flex items-center gap-1.5 text-xs',
                                isConnected ? 'text-green-600 dark:text-green-400' : 'text-muted-foreground'
                            )}
                        >
                            <span
                                className={cn(
                                    'size-1.5 rounded-full',
                                    isConnected ? 'bg-green-500 animate-pulse' : 'bg-muted-foreground/50'
                                )}
                            />
                            {isConnected ? t('connected') : t('disconnected')}
                        </span>
                        {streamError && !isConnected ? (
                            <span className="text-xs text-orange-600 dark:text-orange-400">{t('reconnecting')}</span>
                        ) : null}
                    </div>
                    <div className="text-xs text-muted-foreground">
                        {t('summaryLine', {
                            abnormal: summary.channels_abnormal,
                            total: summary.channels_total,
                        })}
                    </div>
                </div>

                <div className="flex flex-wrap gap-2 text-xs">
                    <SummaryChip label={t('status.circuit_open')} value={summary.circuit_open} tone="red" />
                    <SummaryChip label={t('status.rate_limited')} value={summary.rate_limited} tone="orange" />
                    <SummaryChip label={t('status.disabled')} value={summary.disabled} tone="muted" />
                    <SummaryChip label={t('status.circuit_half_open')} value={summary.half_open} tone="yellow" />
                    <SummaryChip label={t('status.degraded')} value={summary.degraded} tone="amber" />
                    <SummaryChip label={t('status.ok')} value={summary.ok} tone="green" />
                </div>

                <div className="flex flex-wrap items-center gap-2">
                    <Input
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        placeholder={t('searchPlaceholder')}
                        className="h-8 max-w-xs rounded-xl text-sm"
                    />
                    <select
                        value={groupId}
                        onChange={(e) => setGroupId(Number(e.target.value) || 0)}
                        className="h-8 rounded-xl border border-border bg-background px-2 text-sm"
                    >
                        <option value={0}>{t('allGroups')}</option>
                        {(groups ?? []).map((g) => (
                            <option key={g.id ?? g.name} value={g.id ?? 0}>
                                {g.name}
                            </option>
                        ))}
                    </select>
                    <div className="inline-flex rounded-xl border border-border overflow-hidden text-xs">
                        <button
                            type="button"
                            onClick={() => setAbnormalOnly(true)}
                            className={cn(
                                'px-3 h-8 transition-colors',
                                abnormalOnly ? 'bg-primary text-primary-foreground' : 'bg-background hover:bg-muted'
                            )}
                        >
                            {t('filterAbnormal')}
                        </button>
                        <button
                            type="button"
                            onClick={() => setAbnormalOnly(false)}
                            className={cn(
                                'px-3 h-8 transition-colors border-l border-border',
                                !abnormalOnly ? 'bg-primary text-primary-foreground' : 'bg-background hover:bg-muted'
                            )}
                        >
                            {t('filterAll')}
                        </button>
                    </div>
                </div>
            </div>

            {isLoading ? (
                <div className="flex justify-center py-12">
                    <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
            ) : filtered.length === 0 ? (
                <div className="rounded-3xl border border-dashed border-border p-10 text-center text-sm text-muted-foreground">
                    {abnormalOnly ? t('emptyAbnormal') : t('emptyAll')}
                </div>
            ) : (
                <div className="space-y-2">
                    {filtered.map((ch) => (
                        <ChannelRow key={ch.id} channel={ch} />
                    ))}
                </div>
            )}
        </PageWrapper>
    );
}

function SummaryChip({
    label,
    value,
    tone,
}: {
    label: string;
    value: number;
    tone: 'red' | 'orange' | 'yellow' | 'amber' | 'green' | 'muted';
}) {
    const toneClass: Record<typeof tone, string> = {
        red: 'bg-red-500/10 text-red-700 dark:text-red-400',
        orange: 'bg-orange-500/10 text-orange-700 dark:text-orange-400',
        yellow: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400',
        amber: 'bg-amber-500/10 text-amber-700 dark:text-amber-400',
        green: 'bg-green-500/10 text-green-700 dark:text-green-400',
        muted: 'bg-muted text-muted-foreground',
    };
    return (
        <span className={cn('inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 font-medium', toneClass[tone])}>
            <span className="tabular-nums font-semibold">{value}</span>
            <span className="opacity-80">{label}</span>
        </span>
    );
}
