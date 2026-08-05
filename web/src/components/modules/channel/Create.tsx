import { useState } from 'react';
import { useMorphingDialog } from '@/components/ui/morphing-dialog';
import { DialogShell } from '@/components/common/DialogShell';
import { useCreateChannel } from '@/api/endpoints/channel';
import { useTranslations } from 'use-intl';
import { toast } from '@/components/common/Toast';
import {
    ChannelForm,
    emptyChannelForm,
    keyFormToAddPayload,
    normalizeBaseUrls,
    normalizeCooldown,
    normalizeHeaders,
    type ChannelFormData,
} from './Form';

export function CreateDialogContent() {
    const { setIsOpen } = useMorphingDialog();
    const createChannel = useCreateChannel();
    const [formData, setFormData] = useState<ChannelFormData>(() => emptyChannelForm());
    const t = useTranslations('channel.create');

    const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        const normalizedKeys = formData.allow_empty_key
            ? []
            : formData.keys.filter((k) => k.channel_key.trim()).map(keyFormToAddPayload);

        createChannel.mutate(
            {
                name: formData.name,
                type: formData.type,
                enabled: formData.enabled,
                base_urls: normalizeBaseUrls(formData.base_urls),
                keys: normalizedKeys,
                model: formData.model,
                custom_model: formData.custom_model,
                proxy: formData.proxy,
                auto_sync: formData.auto_sync,
                auto_group: formData.auto_group,
                custom_header: normalizeHeaders(formData.custom_header),
                channel_proxy: formData.channel_proxy.trim(),
                param_override: formData.param_override.trim(),
                match_regex: formData.match_regex.trim(),
                key_mode: formData.key_mode,
                rate_limit_cooldown_sec: normalizeCooldown(formData.rate_limit_cooldown_sec),
                allow_empty_key: formData.allow_empty_key,
            },
            {
                onSuccess: () => {
                    setFormData(emptyChannelForm());
                    setIsOpen(false);
                },
                onError: (error) => {
                    toast.error(t('createFailed'), { description: error.message });
                },
            },
        );
    };

    return (
        <DialogShell title={t('dialogTitle')} scroll="body">
            <ChannelForm
                formData={formData}
                onFormDataChange={setFormData}
                onSubmit={handleSubmit}
                isPending={createChannel.isPending}
                submitText={t('submit')}
                pendingText={t('submitting')}
                idPrefix="create-channel"
            />
        </DialogShell>
    );
}
