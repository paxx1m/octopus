import { useState } from 'react';
import {
    MorphingDialogClose,
    MorphingDialogTitle,
    MorphingDialogDescription,
    useMorphingDialog,
} from '@/components/ui/morphing-dialog';
import { useCreateChannel } from '@/api/endpoints/channel';
import { useTranslations } from 'use-intl';
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
            },
        );
    };

    return (
        <div className="w-screen max-w-full md:max-w-xl h-full min-h-0 flex flex-col">
            <MorphingDialogTitle className="shrink-0">
                <header className="mb-6 flex items-center justify-between">
                    <h2 className="text-2xl font-bold text-card-foreground">{t('dialogTitle')}</h2>
                    <MorphingDialogClose
                        className="relative right-0 top-0"
                        variants={{
                            initial: { opacity: 0, scale: 0.8 },
                            animate: { opacity: 1, scale: 1 },
                            exit: { opacity: 0, scale: 0.8 },
                        }}
                    />
                </header>
            </MorphingDialogTitle>
            <MorphingDialogDescription className="flex-1 min-h-0 overflow-y-auto">
                <ChannelForm
                    formData={formData}
                    onFormDataChange={setFormData}
                    onSubmit={handleSubmit}
                    isPending={createChannel.isPending}
                    submitText={t('submit')}
                    pendingText={t('submitting')}
                    idPrefix="create-channel"
                />
            </MorphingDialogDescription>
        </div>
    );
}
