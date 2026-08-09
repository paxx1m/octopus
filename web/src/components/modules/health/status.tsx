import { cn } from '@/lib/utils';
import type { HealthStatus } from '@/api/endpoints/health';
import { formatRemaining } from '@/api/endpoints/health';
import { Badge } from '@/components/ui/badge';
import { useTranslations } from 'use-intl';

const STATUS_CLASS: Record<HealthStatus, string> = {
    disabled: 'bg-muted text-muted-foreground',
    circuit_open: 'bg-red-500/15 text-red-700 dark:text-red-400',
    rate_limited: 'bg-orange-500/15 text-orange-700 dark:text-orange-400',
    circuit_half_open: 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-400',
    degraded: 'bg-amber-500/15 text-amber-700 dark:text-amber-400',
    ok: 'bg-green-500/15 text-green-700 dark:text-green-400',
};

export function StatusBadge({ status, className }: { status: HealthStatus; className?: string }) {
    const t = useTranslations('health.status');
    return (
        <Badge variant="secondary" className={cn('h-5 px-1.5 text-[10px]', STATUS_CLASS[status] ?? STATUS_CLASS.ok, className)}>
            {t(status)}
        </Badge>
    );
}

export function Countdown({ sec }: { sec?: number }) {
    if (sec == null || sec < 0) return null;
    return (
        <span className="font-mono text-xs text-muted-foreground tabular-nums">
            {formatRemaining(sec)}
        </span>
    );
}
