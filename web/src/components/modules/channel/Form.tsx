import { useEffect } from 'react';
import { useTranslations } from 'use-intl';
import { Plus, X } from 'lucide-react';
import { ChannelType, type Channel } from '@/api/endpoints/channel';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { defaultKeyItem, type ChannelFormData } from './form-utils';
import { FormAdvancedSection } from './FormAdvancedSection';
import { FormKeysSection } from './FormKeysSection';
import { FormModelsSection } from './FormModelsSection';

export type { ChannelFormData, ChannelKeyFormItem } from './form-utils';
export {
    channelToFormData,
    cooldownPatch,
    defaultKeyItem,
    emptyChannelForm,
    keyFormToAddPayload,
    keyToFormItem,
    maskChannelKey,
    normalizeBaseUrls,
    normalizeCooldown,
    normalizeHeaders,
    normalizeWeight,
    parseKeyLine,
    parseKeyLines,
} from './form-utils';

export interface ChannelFormProps {
    formData: ChannelFormData;
    onFormDataChange: (data: ChannelFormData) => void;
    onSubmit: (event: React.FormEvent<HTMLFormElement>) => void;
    isPending: boolean;
    submitText: string;
    pendingText: string;
    onCancel?: () => void;
    cancelText?: string;
    idPrefix?: string;
}

export function ChannelForm({
    formData,
    onFormDataChange,
    onSubmit,
    isPending,
    submitText,
    pendingText,
    onCancel,
    cancelText,
    idPrefix = 'channel',
}: ChannelFormProps) {
    const t = useTranslations('channel.form');
    const tc = useTranslations('common');

    const baseUrlsLen = formData.base_urls?.length ?? 0;
    const keysLen = formData.keys?.length ?? 0;
    const headersLen = formData.custom_header?.length ?? 0;
    const allowEmptyKey = formData.allow_empty_key;

    useEffect(() => {
        if (baseUrlsLen === 0) {
            onFormDataChange({ ...formData, base_urls: [{ url: '', delay: 0 }] });
            return;
        }
        if (!allowEmptyKey && keysLen === 0) {
            onFormDataChange({ ...formData, keys: [defaultKeyItem()] });
            return;
        }
        if (headersLen === 0) {
            onFormDataChange({
                ...formData,
                custom_header: [{ header_key: '', header_value: '' }],
            });
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [baseUrlsLen, keysLen, headersLen, allowEmptyKey, onFormDataChange]);

    const handleAddBaseUrl = () => {
        onFormDataChange({
            ...formData,
            base_urls: [...(formData.base_urls ?? []), { url: '', delay: 0 }],
        });
    };

    const handleUpdateBaseUrl = (idx: number, patch: Partial<Channel['base_urls'][number]>) => {
        const next = (formData.base_urls ?? []).map((u, i) =>
            i === idx ? { ...u, ...patch } : u,
        );
        onFormDataChange({ ...formData, base_urls: next });
    };

    const handleRemoveBaseUrl = (idx: number) => {
        const curr = formData.base_urls ?? [];
        if (curr.length <= 1) return;
        onFormDataChange({ ...formData, base_urls: curr.filter((_, i) => i !== idx) });
    };

    return (
        <form onSubmit={onSubmit} className="min-w-0 space-y-4 px-1">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                    <label
                        htmlFor={`${idPrefix}-name`}
                        className="text-sm font-medium text-card-foreground"
                    >
                        {t('name')}
                    </label>
                    <Input
                        className="rounded-xl"
                        id={`${idPrefix}-name`}
                        type="text"
                        value={formData.name}
                        onChange={(event) =>
                            onFormDataChange({ ...formData, name: event.target.value })
                        }
                        required
                    />
                </div>

                <div className="space-y-2">
                    <label
                        htmlFor={`${idPrefix}-type`}
                        className="text-sm font-medium text-card-foreground"
                    >
                        {t('type')}
                    </label>
                    <Select
                        value={String(formData.type)}
                        onValueChange={(value) =>
                            onFormDataChange({ ...formData, type: value as ChannelType })
                        }
                    >
                        <SelectTrigger
                            id={`${idPrefix}-type`}
                            className="w-full rounded-xl border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            <SelectItem
                                className="rounded-xl"
                                value={String(ChannelType.OpenAIChat)}
                            >
                                {t('typeOpenAIChat')}
                            </SelectItem>
                            <SelectItem
                                className="rounded-xl"
                                value={String(ChannelType.OpenAIResponse)}
                            >
                                {t('typeOpenAIResponse')}
                            </SelectItem>
                            <SelectItem
                                className="rounded-xl"
                                value={String(ChannelType.Anthropic)}
                            >
                                {t('typeAnthropic')}
                            </SelectItem>
                            <SelectItem className="rounded-xl" value={String(ChannelType.Gemini)}>
                                {t('typeGemini')}
                            </SelectItem>
                            <SelectItem
                                className="rounded-xl"
                                value={String(ChannelType.Volcengine)}
                            >
                                {t('typeVolcengine')}
                            </SelectItem>
                            <SelectItem
                                className="rounded-xl"
                                value={String(ChannelType.OpenAIEmbedding)}
                            >
                                {t('typeOpenAIEmbedding')}
                            </SelectItem>
                        </SelectContent>
                    </Select>
                </div>
            </div>

            <div className="space-y-2">
                <div className="flex items-center justify-between">
                    <label className="text-sm font-medium text-card-foreground">
                        {t('baseUrls')}{' '}
                        {formData.base_urls.length > 0 ? `(${formData.base_urls.length})` : ''}
                    </label>
                    <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={handleAddBaseUrl}
                        className="h-6 px-2 text-xs text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground"
                    >
                        <Plus className="mr-1 h-3 w-3" />
                        {t('add')}
                    </Button>
                </div>
                <div className="space-y-2">
                    {(formData.base_urls ?? []).map((u, idx) => (
                        <div key={`baseurl-${idx}`} className="flex items-center gap-2">
                            <Input
                                id={`${idPrefix}-base-${idx}`}
                                type="url"
                                value={u.url}
                                onChange={(e) => handleUpdateBaseUrl(idx, { url: e.target.value })}
                                placeholder={t('baseUrlUrl')}
                                required={idx === 0}
                                className="flex-1 rounded-xl"
                            />
                            <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={() => handleRemoveBaseUrl(idx)}
                                disabled={(formData.base_urls ?? []).length <= 1}
                                className="h-8 w-8 rounded-xl p-0 text-muted-foreground hover:bg-transparent hover:text-destructive disabled:opacity-40"
                                title={tc("remove")}
                            >
                                <X className="h-4 w-4" />
                            </Button>
                        </div>
                    ))}
                </div>
            </div>

            <div className="flex items-center justify-between rounded-xl border border-border/50 bg-muted/20 px-4 py-3">
                <div className="space-y-0.5">
                    <label className="text-sm font-medium text-card-foreground">
                        {t('allowEmptyKey')}
                    </label>
                    <p className="text-xs text-muted-foreground">{t('allowEmptyKeyHint')}</p>
                </div>
                <Switch
                    checked={formData.allow_empty_key}
                    onCheckedChange={(checked) =>
                        onFormDataChange({ ...formData, allow_empty_key: checked })
                    }
                />
            </div>

            <FormKeysSection
                formData={formData}
                onFormDataChange={onFormDataChange}
                idPrefix={idPrefix}
            />

            <FormModelsSection
                formData={formData}
                onFormDataChange={onFormDataChange}
                idPrefix={idPrefix}
            />

            <FormAdvancedSection
                formData={formData}
                onFormDataChange={onFormDataChange}
                idPrefix={idPrefix}
            />

            <div className="flex flex-wrap items-center justify-between gap-4 rounded-xl border border-border/50 bg-muted/20 p-4">
                <label className="flex cursor-pointer items-center gap-2">
                    <Switch
                        checked={formData.enabled}
                        onCheckedChange={(checked) =>
                            onFormDataChange({ ...formData, enabled: checked })
                        }
                    />
                    <span className="text-sm font-medium text-card-foreground">{t('enabled')}</span>
                </label>
                <div className="flex items-center gap-6">
                    <label className="flex cursor-pointer items-center gap-2">
                        <Switch
                            checked={formData.proxy}
                            onCheckedChange={(checked) =>
                                onFormDataChange({ ...formData, proxy: checked })
                            }
                        />
                        <span className="text-sm text-card-foreground">{t('proxy')}</span>
                    </label>
                    <label className="flex cursor-pointer items-center gap-2">
                        <Switch
                            checked={formData.auto_sync}
                            onCheckedChange={(checked) =>
                                onFormDataChange({ ...formData, auto_sync: checked })
                            }
                        />
                        <span className="text-sm text-card-foreground">{t('autoSync')}</span>
                    </label>
                </div>
            </div>

            <div className={`flex flex-col gap-3 pt-2 ${onCancel ? 'sm:flex-row' : ''}`}>
                {onCancel && cancelText && (
                    <Button
                        type="button"
                        variant="secondary"
                        onClick={onCancel}
                        className="h-12 w-full rounded-2xl sm:flex-1"
                    >
                        {cancelText}
                    </Button>
                )}
                <Button
                    type="submit"
                    disabled={isPending}
                    className="h-12 w-full rounded-2xl sm:flex-1"
                >
                    {isPending ? pendingText : submitText}
                </Button>
            </div>
        </form>
    );
}
