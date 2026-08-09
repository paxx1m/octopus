import { useState } from 'react';
import { ChevronDown, ChevronRight, KeyRound } from 'lucide-react';
import type { HealthChannel } from '@/api/endpoints/health';
import { StatusBadge } from './status';
import { KeyDetail } from './KeyDetail';
import { useTranslations } from 'use-intl';
import { cn } from '@/lib/utils';

export function ChannelRow({ channel }: { channel: HealthChannel }) {
    const t = useTranslations('health');
    const keys = channel.keys ?? [];
    const keyCount = keys.length;
    // 0 or 1 key: no expand panel — single key details always inline when present
    const expandable = keyCount > 1;
    const [open, setOpen] = useState(false);

    return (
        <article className="rounded-3xl border border-border bg-card text-card-foreground overflow-hidden">
            <button
                type="button"
                disabled={!expandable}
                onClick={() => expandable && setOpen((v) => !v)}
                className={cn(
                    'w-full flex items-center gap-3 p-4 text-left transition-colors',
                    expandable && 'hover:bg-muted/40 cursor-pointer',
                    !expandable && 'cursor-default'
                )}
            >
                <div className="w-5 shrink-0 flex justify-center text-muted-foreground">
                    {expandable ? (
                        open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />
                    ) : (
                        <span className="size-4" />
                    )}
                </div>

                <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                        <h3 className="text-base font-semibold truncate">{channel.name}</h3>
                        <StatusBadge status={channel.status} />
                        {!channel.enabled ? (
                            <span className="text-[10px] text-muted-foreground">{t('channelDisabled')}</span>
                        ) : null}
                    </div>
                    <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
                        <span className="inline-flex items-center gap-1">
                            <KeyRound className="size-3.5" />
                            {channel.allow_empty_key && keyCount === 0
                                ? t('emptyKeyMode')
                                : t('keysAvailable', {
                                      available: channel.available_keys,
                                      total: channel.total_keys,
                                  })}
                        </span>
                    </div>
                </div>
            </button>

            {/* single key: inline detail only when that key is abnormal or has model circuits */}
            {!expandable && keyCount === 1 && (keys[0].status !== 'ok' || (keys[0].models?.length ?? 0) > 0) ? (
                <div className="px-4 pb-4 pl-12">
                    <KeyDetail healthKey={keys[0]} compact />
                </div>
            ) : null}

            {/* multi key: collapsible panel */}
            {expandable && open ? (
                <div className="border-t border-border/60 px-4 py-3 pl-12 space-y-2 bg-muted/20">
                    {keys.map((k) => (
                        <KeyDetail key={k.id} healthKey={k} />
                    ))}
                </div>
            ) : null}
        </article>
    );
}
