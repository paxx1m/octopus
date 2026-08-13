import { useTranslations } from 'use-intl';
import { DollarSign, Clock, RefreshCw } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { SettingKey } from '@/api/endpoints/setting';
import { useLastUpdateTime } from '@/api/endpoints/model';
import { apiClient } from '@/api/client';
import { SettingCard, SettingRow } from '@/components/common/SettingCard';
import { useSettingField } from '@/hooks/useSettingField';
import { useToastMutation } from '@/api/mutation-helpers';
import { formatTimestamp } from '@/lib/metrics';

export function SettingLLMPrice() {
    const t = useTranslations('setting');
    const updateInterval = useSettingField(SettingKey.ModelInfoUpdateInterval);
    const { data: lastUpdateTime } = useLastUpdateTime();

    const updatePrice = useToastMutation({
        name: '模型价格更新',
        mutationFn: () => apiClient.post<null>('/api/v1/model/update-price', {}),
        successMsg: t('llmPrice.updateSuccess'),
        errorMsg: t('llmPrice.updateFailed'),
        invalidate: [['models', 'last-update-time']],
    });

    return (
        <SettingCard icon={<DollarSign className="h-5 w-5" />} title={t('llmPrice.title')}>
            <SettingRow icon={<Clock className="h-5 w-5 text-muted-foreground" />} label={t('llmPrice.updateInterval.label')}>
                <Input
                    type="number"
                    value={updateInterval.value}
                    onChange={(e) => updateInterval.setValue(e.target.value)}
                    onBlur={updateInterval.save}
                    placeholder={t('llmPrice.updateInterval.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>

            <SettingRow
                label={
                    <div className="flex flex-col gap-1">
                        <div className="flex items-center gap-3">
                            <RefreshCw className="h-5 w-5 text-muted-foreground" />
                            <span className="text-sm font-medium">{t('llmPrice.manualUpdate.label')}</span>
                        </div>
                        <span className="text-xs text-muted-foreground ml-8">
                            {t('llmPrice.lastUpdate')}: {formatTimestamp(lastUpdateTime, t('llmPrice.neverUpdated'))}
                        </span>
                    </div>
                }
            >
                <Button variant="outline" size="sm" onClick={() => updatePrice.mutate(undefined)} disabled={updatePrice.isPending} className="rounded-xl">
                    {updatePrice.isPending ? t('llmPrice.manualUpdate.updating') : t('llmPrice.manualUpdate.button')}
                </Button>
            </SettingRow>
        </SettingCard>
    );
}
