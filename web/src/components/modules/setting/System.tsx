import { useMemo, useState } from 'react';
import { useTranslations } from 'use-intl';
import { Monitor, Globe, Clock, Shield, HelpCircle, X } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { SettingKey } from '@/api/endpoints/setting';
import { SettingCard, SettingRow } from '@/components/common/SettingCard';
import { useSettingField } from '@/hooks/useSettingField';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';

export function SettingSystem() {
    const t = useTranslations('setting');

    const proxyUrl = useSettingField(SettingKey.ProxyURL);
    const statsSaveInterval = useSettingField(SettingKey.StatsSaveInterval);
    const keyRateLimitCooldown = useSettingField(SettingKey.ChannelKeyRateLimitCooldown);

    // CORS 白名单有特殊的增删交互，保留独立状态管理
    const corsField = useSettingField(SettingKey.CORSAllowOrigins);
    const [corsInputValue, setCorsInputValue] = useState('');

    const corsAllowOriginsList = useMemo(() => {
        const value = corsField.value.trim();
        if (!value) return [];
        if (value === '*') return ['*'];
        return Array.from(new Set(
            value
                .split(/[,\n，]/)
                .map(item => item.trim())
                .filter(Boolean)
        ));
    }, [corsField.value]);

    const corsAllowOriginsDisplay = useMemo(
        () => (corsAllowOriginsList.length > 0 ? corsAllowOriginsList.join(', ') : t('corsAllowOrigins.hint')),
        [corsAllowOriginsList, t]
    );

    const saveCorsAllowOrigins = (origins: string[]) => {
        const normalizedOrigins = Array.from(new Set(
            origins
                .map(origin => origin.trim())
                .filter(Boolean)
        ));
        const normalizedValue = normalizedOrigins.includes('*') ? '*' : normalizedOrigins.join(',');
        corsField.saveValue(normalizedValue);
    };

    const handleAddCorsOrigin = () => {
        const newOrigins = Array.from(new Set(
            corsInputValue
                .split(/[,\n，]/)
                .map(item => item.trim())
                .filter(Boolean)
        ));
        if (newOrigins.length === 0) return;

        if (newOrigins.includes('*')) {
            saveCorsAllowOrigins(['*']);
            setCorsInputValue('');
            return;
        }

        const base = corsAllowOriginsList.includes('*') ? [] : corsAllowOriginsList;
        const merged = Array.from(new Set([...base, ...newOrigins]));
        saveCorsAllowOrigins(merged);
        setCorsInputValue('');
    };

    const handleRemoveCorsOrigin = (originToRemove: string) => {
        const nextOrigins = corsAllowOriginsList.filter(origin => origin !== originToRemove);
        saveCorsAllowOrigins(nextOrigins);
    };

    return (
        <SettingCard icon={<Monitor className="h-5 w-5" />} title={t('system')}>
            <SettingRow icon={<Globe className="h-5 w-5 text-muted-foreground" />} label={t('proxyUrl.label')}>
                <Input
                    value={proxyUrl.value}
                    onChange={(e) => proxyUrl.setValue(e.target.value)}
                    onBlur={proxyUrl.save}
                    placeholder={t('proxyUrl.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>

            <SettingRow icon={<Clock className="h-5 w-5 text-muted-foreground" />} label={t('statsSaveInterval.label')}>
                <Input
                    type="number"
                    value={statsSaveInterval.value}
                    onChange={(e) => statsSaveInterval.setValue(e.target.value)}
                    onBlur={statsSaveInterval.save}
                    placeholder={t('statsSaveInterval.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>

            <SettingRow
                icon={
                    <>
                        <Clock className="h-5 w-5 text-muted-foreground" />
                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <HelpCircle className="size-4 text-muted-foreground cursor-help" />
                                </TooltipTrigger>
                                <TooltipContent>
                                    {t('channelKeyRateLimitCooldown.hint')}
                                </TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                    </>
                }
                label={t('channelKeyRateLimitCooldown.label')}
            >
                <Input
                    type="number"
                    value={keyRateLimitCooldown.value}
                    onChange={(e) => keyRateLimitCooldown.setValue(e.target.value)}
                    onBlur={keyRateLimitCooldown.save}
                    placeholder={t('channelKeyRateLimitCooldown.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>

            <SettingRow
                icon={
                    <>
                        <Shield className="h-5 w-5 text-muted-foreground" />
                        <TooltipProvider>
                            <Tooltip>
                                <TooltipTrigger asChild>
                                    <HelpCircle className="size-4 text-muted-foreground cursor-help" />
                                </TooltipTrigger>
                                <TooltipContent>
                                    {t('corsAllowOrigins.hint')}
                                    <br />
                                    {t('corsAllowOrigins.example')}
                                </TooltipContent>
                            </Tooltip>
                        </TooltipProvider>
                    </>
                }
                label={t('corsAllowOrigins.label')}
            >
                <Popover>
                    <PopoverTrigger asChild>
                        <button
                            type="button"
                            className="border-input focus-visible:border-ring focus-visible:ring-ring/50 w-48 min-h-9 rounded-xl border bg-transparent px-3 py-2 text-left text-sm shadow-xs transition-[color,box-shadow] outline-none focus-visible:ring-[3px]"
                            title={corsAllowOriginsDisplay}
                        >
                            <span className={`block overflow-hidden text-ellipsis whitespace-nowrap ${corsAllowOriginsList.length === 0 ? 'text-muted-foreground' : ''}`}>
                                {corsAllowOriginsDisplay}
                            </span>
                        </button>
                    </PopoverTrigger>
                    <PopoverContent className="w-72 space-y-2 rounded-3xl p-3 bg-card">
                        <Input
                            value={corsInputValue}
                            onChange={(e) => setCorsInputValue(e.target.value)}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter') {
                                    e.preventDefault();
                                    handleAddCorsOrigin();
                                }
                            }}
                            placeholder={t('corsAllowOrigins.example')}
                            className="h-9 rounded-xl"
                            autoFocus
                        />
                        <div className="max-h-48 space-y-1 overflow-y-auto">
                            {corsAllowOriginsList.length > 0 && (
                                corsAllowOriginsList.map((origin) => (
                                    <div key={origin} className="flex items-center justify-between gap-2 rounded-xl border border-border/60 px-2 py-1">
                                        <span className="break-all text-xs leading-5">{origin}</span>
                                        <button
                                            type="button"
                                            onClick={() => handleRemoveCorsOrigin(origin)}
                                            className="text-muted-foreground transition-colors hover:text-destructive"
                                            aria-label={`remove ${origin}`}
                                        >
                                            <X className="size-4" />
                                        </button>
                                    </div>
                                ))
                            )}
                        </div>
                    </PopoverContent>
                </Popover>
            </SettingRow>
        </SettingCard>
    );
}
