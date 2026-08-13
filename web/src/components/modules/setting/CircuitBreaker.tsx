import { useTranslations } from 'use-intl';
import { Zap, Hash, Timer, TimerOff, HelpCircle } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { SettingKey } from '@/api/endpoints/setting';
import { SettingCard, SettingRow } from '@/components/common/SettingCard';
import { useSettingField } from '@/hooks/useSettingField';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';

export function SettingCircuitBreaker() {
    const t = useTranslations('setting');

    const threshold = useSettingField(SettingKey.CircuitBreakerThreshold);
    const cooldown = useSettingField(SettingKey.CircuitBreakerCooldown);
    const maxCooldown = useSettingField(SettingKey.CircuitBreakerMaxCooldown);

    return (
        <SettingCard
            icon={
                <>
                    <Zap className="h-5 w-5" />
                    <TooltipProvider>
                        <Tooltip>
                            <TooltipTrigger asChild>
                                <HelpCircle className="size-4 text-muted-foreground cursor-help" />
                            </TooltipTrigger>
                            <TooltipContent>
                                {t('circuitBreaker.hint')}
                            </TooltipContent>
                        </Tooltip>
                    </TooltipProvider>
                </>
            }
            title={t('circuitBreaker.title')}
        >
            <SettingRow icon={<Hash className="h-5 w-5 text-muted-foreground" />} label={t('circuitBreaker.threshold.label')}>
                <Input
                    type="number"
                    value={threshold.value}
                    onChange={(e) => threshold.setValue(e.target.value)}
                    onBlur={threshold.save}
                    placeholder={t('circuitBreaker.threshold.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>

            <SettingRow icon={<Timer className="h-5 w-5 text-muted-foreground" />} label={t('circuitBreaker.cooldown.label')}>
                <Input
                    type="number"
                    value={cooldown.value}
                    onChange={(e) => cooldown.setValue(e.target.value)}
                    onBlur={cooldown.save}
                    placeholder={t('circuitBreaker.cooldown.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>

            <SettingRow icon={<TimerOff className="h-5 w-5 text-muted-foreground" />} label={t('circuitBreaker.maxCooldown.label')}>
                <Input
                    type="number"
                    value={maxCooldown.value}
                    onChange={(e) => maxCooldown.setValue(e.target.value)}
                    onBlur={maxCooldown.save}
                    placeholder={t('circuitBreaker.maxCooldown.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>
        </SettingCard>
    );
}
