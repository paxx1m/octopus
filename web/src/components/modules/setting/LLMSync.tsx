import { useTranslations } from 'use-intl';
import { RefreshCw, Clock } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { SettingKey } from '@/api/endpoints/setting';
import { useLastSyncTime } from '@/api/endpoints/channel';
import { apiClient } from '@/api/client';
import { SettingCard, SettingRow } from '@/components/common/SettingCard';
import { useSettingField } from '@/hooks/useSettingField';
import { useToastMutation } from '@/api/mutation-helpers';
import { formatTimestamp } from '@/lib/metrics';

export function SettingLLMSync() {
    const t = useTranslations('setting');
    const syncInterval = useSettingField(SettingKey.SyncLLMInterval);
    const { data: lastSyncTime } = useLastSyncTime();

    const syncChannel = useToastMutation({
        name: '渠道同步',
        mutationFn: () => apiClient.post<null>('/api/v1/channel/sync'),
        successMsg: t('llmSync.syncSuccess'),
        errorMsg: t('llmSync.syncFailed'),
        invalidate: [['channels', 'last-sync-time']],
    });

    return (
        <SettingCard icon={<RefreshCw className="h-5 w-5" />} title={t('llmSync.title')}>
            <SettingRow icon={<Clock className="h-5 w-5 text-muted-foreground" />} label={t('llmSync.syncInterval.label')}>
                <Input
                    type="number"
                    value={syncInterval.value}
                    onChange={(e) => syncInterval.setValue(e.target.value)}
                    onBlur={syncInterval.save}
                    placeholder={t('llmSync.syncInterval.placeholder')}
                    className="w-48 rounded-xl"
                />
            </SettingRow>

            <SettingRow
                label={
                    <div className="flex flex-col gap-1">
                        <div className="flex items-center gap-3">
                            <RefreshCw className="h-5 w-5 text-muted-foreground" />
                            <span className="text-sm font-medium">{t('llmSync.manualSync.label')}</span>
                        </div>
                        <span className="text-xs text-muted-foreground ml-8">
                            {t('llmSync.lastSync')}: {formatTimestamp(lastSyncTime, t('llmSync.neverSynced'))}
                        </span>
                    </div>
                }
            >
                <Button variant="outline" size="sm" onClick={() => syncChannel.mutate(undefined)} disabled={syncChannel.isPending} className="rounded-xl">
                    {syncChannel.isPending ? t('llmSync.manualSync.syncing') : t('llmSync.manualSync.button')}
                </Button>
            </SettingRow>
        </SettingCard>
    );
}
