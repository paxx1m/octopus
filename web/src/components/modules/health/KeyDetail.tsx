import type { HealthKey } from '@/api/endpoints/health';
import { StatusBadge, Countdown } from './status';
import { useTranslations } from 'use-intl';

export function KeyDetail({ healthKey, compact }: { healthKey: HealthKey; compact?: boolean }) {
    const t = useTranslations('health');
    const models = healthKey.models ?? [];

    return (
        <div className={compact ? 'space-y-1.5' : 'space-y-2 rounded-2xl border border-border/60 bg-background/60 p-3'}>
            <div className="flex flex-wrap items-center gap-2 min-w-0">
                <span className="font-mono text-sm truncate">
                    {healthKey.masked || t('emptyKey')}
                </span>
                {healthKey.remark ? (
                    <span className="text-xs text-muted-foreground truncate max-w-28" title={healthKey.remark}>
                        {healthKey.remark}
                    </span>
                ) : null}
                <StatusBadge status={healthKey.status} />
                {healthKey.status_code > 0 ? (
                    <span className="text-[10px] text-muted-foreground font-mono">{healthKey.status_code}</span>
                ) : null}
                {healthKey.rate_limit && healthKey.rate_limit.remaining_sec > 0 ? (
                    <span className="inline-flex items-center gap-1 text-xs text-orange-600 dark:text-orange-400">
                        <span>429</span>
                        <Countdown sec={healthKey.rate_limit.remaining_sec} />
                    </span>
                ) : null}
            </div>

            {models.length > 0 ? (
                <ul className="space-y-1 pl-1 border-l border-border/50 ml-1">
                    {models.map((m) => (
                        <li key={m.name} className="flex flex-wrap items-center gap-2 pl-2 text-sm">
                            <span className="font-mono text-xs truncate min-w-0 max-w-[40%]">{m.name}</span>
                            <StatusBadge status={m.status} />
                            {m.circuit?.state === 'half_open' ? (
                                <span className="text-[10px] text-yellow-600 dark:text-yellow-400">{t('probing')}</span>
                            ) : null}
                            {m.circuit?.state === 'open' && (m.circuit.remaining_sec ?? 0) > 0 ? (
                                <Countdown sec={m.circuit.remaining_sec} />
                            ) : null}
                            {(m.circuit?.consecutive_failures ?? 0) > 0 && m.circuit?.state === 'closed' ? (
                                <span className="text-[10px] text-muted-foreground">
                                    {t('failures', { count: m.circuit!.consecutive_failures })}
                                </span>
                            ) : null}
                        </li>
                    ))}
                </ul>
            ) : null}
        </div>
    );
}
