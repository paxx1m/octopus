import { useTranslations } from 'use-intl';
import { ScrollText, Calendar, Trash2 } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { SettingKey } from '@/api/endpoints/setting';
import { useClearLogs } from '@/api/endpoints/log';
import { SettingCard, SettingRow } from '@/components/common/SettingCard';
import { useSettingField } from '@/hooks/useSettingField';
import { toast } from '@/components/common/Toast';
import { useState } from 'react';

export function SettingLog() {
    const t = useTranslations('setting');
    const keepPeriod = useSettingField(SettingKey.RelayLogKeepPeriod);
    const clearLogs = useClearLogs();
    const [isClearing, setIsClearing] = useState(false);

    // enabled 是 Switch，交互方式与 onBlur 不同，保留独立状态
    const enabledField = useSettingField(SettingKey.RelayLogKeepEnabled);
    const enabled = enabledField.value === 'true';

    const handleEnabledChange = (checked: boolean) => {
        const value = checked ? 'true' : 'false';
        enabledField.saveValue(value);
    };

    const handleClearLogs = () => {
        setIsClearing(true);
        clearLogs.mutate(undefined, {
            onSuccess: () => {
                toast.success(t('log.clearSuccess'));
                setIsClearing(false);
            },
            onError: () => {
                toast.error(t('log.clearFailed'));
                setIsClearing(false);
            },
        });
    };

    return (
        <SettingCard icon={<ScrollText className="h-5 w-5" />} title={t('log.title')}>
            <SettingRow icon={<ScrollText className="h-5 w-5 text-muted-foreground" />} label={t('log.enabled.label')}>
                <Switch checked={enabled} onCheckedChange={handleEnabledChange} />
            </SettingRow>

            <SettingRow icon={<Calendar className="h-5 w-5 text-muted-foreground" />} label={t('log.keepPeriod.label')}>
                <Input
                    type="number"
                    value={keepPeriod.value}
                    onChange={(e) => keepPeriod.setValue(e.target.value)}
                    onBlur={keepPeriod.save}
                    placeholder={t('log.keepPeriod.placeholder')}
                    className="w-48 rounded-xl"
                    disabled={!enabled}
                />
            </SettingRow>

            <SettingRow icon={<Trash2 className="h-5 w-5 text-muted-foreground" />} label={t('log.clear.label')}>
                <Button variant="destructive" size="sm" onClick={handleClearLogs} disabled={isClearing} className="rounded-xl">
                    {isClearing ? t('log.clear.clearing') : t('log.clear.button')}
                </Button>
            </SettingRow>
        </SettingCard>
    );
}
