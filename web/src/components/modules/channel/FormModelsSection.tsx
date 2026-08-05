import { useRef, useState } from 'react';
import { useTranslations } from 'use-intl';
import { Plus, RefreshCw, X } from 'lucide-react';
import { useFetchModel } from '@/api/endpoints/channel';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { toast } from '@/components/common/Toast';
import type { ChannelFormData } from './form-utils';

type Props = {
    formData: ChannelFormData;
    onFormDataChange: (data: ChannelFormData) => void;
    idPrefix: string;
};

export function FormModelsSection({ formData, onFormDataChange, idPrefix }: Props) {
    const t = useTranslations('channel.form');
    const [inputValue, setInputValue] = useState('');
    const inputRef = useRef<HTMLInputElement>(null);
    const fetchModel = useFetchModel();

    const autoModels = formData.model
        ? formData.model
              .split(',')
              .map((m) => m.trim())
              .filter(Boolean)
        : [];
    const customModels = formData.custom_model
        ? formData.custom_model
              .split(',')
              .map((m) => m.trim())
              .filter(Boolean)
        : [];

    const effectiveKey =
        formData.keys.find((k) => k.enabled && k.channel_key.trim())?.channel_key.trim() || '';

    const updateModels = (nextAuto: string[], nextCustom: string[]) => {
        const model = nextAuto.join(',');
        const custom_model = nextCustom.join(',');
        if (formData.model === model && formData.custom_model === custom_model) return;
        onFormDataChange({ ...formData, model, custom_model });
    };

    const handleRefreshModels = () => {
        if (!formData.base_urls?.[0]?.url) return;
        if (!formData.allow_empty_key && !effectiveKey) return;
        fetchModel.mutate(
            {
                type: formData.type,
                base_urls: formData.base_urls,
                keys: formData.keys
                    .filter((k) => k.channel_key.trim())
                    .map((k) => ({ enabled: k.enabled, channel_key: k.channel_key.trim() })),
                proxy: formData.proxy,
                channel_proxy: formData.channel_proxy?.trim() || null,
                match_regex: formData.match_regex.trim() || null,
                custom_header: formData.custom_header?.filter((h) => h.header_key.trim()) || [],
            },
            {
                onSuccess: (data) => {
                    if (data && data.length > 0) {
                        const nextAuto = Array.from(
                            new Set(
                                [...autoModels, ...data]
                                    .map((m) => m.trim())
                                    .filter(Boolean),
                            ),
                        );
                        updateModels(nextAuto, customModels);
                        toast.success(t('modelRefreshSuccess'));
                    } else {
                        toast.warning(t('modelRefreshEmpty'));
                    }
                },
                onError: (error) => {
                    const errorMessage = error instanceof Error ? error.message : String(error);
                    toast.error(t('modelRefreshFailed'), { description: errorMessage });
                },
            },
        );
    };

    const handleAddModel = (model: string) => {
        const trimmedModel = model.trim();
        if (
            trimmedModel &&
            !customModels.includes(trimmedModel) &&
            !autoModels.includes(trimmedModel)
        ) {
            updateModels(autoModels, [...customModels, trimmedModel]);
        }
        setInputValue('');
    };

    const handleInputKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            if (inputValue.trim()) handleAddModel(inputValue);
        }
    };

    return (
        <div className="space-y-2">
            <div className="flex items-center justify-between">
                <label className="text-sm font-medium text-card-foreground">{t('model')}</label>
                <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={handleRefreshModels}
                    disabled={
                        !formData.base_urls?.[0]?.url ||
                        (!formData.allow_empty_key && !effectiveKey) ||
                        fetchModel.isPending
                    }
                    className="h-6 px-2 text-xs text-muted-foreground/50 hover:bg-transparent hover:text-muted-foreground"
                >
                    <RefreshCw
                        className={`mr-1 h-3 w-3 ${fetchModel.isPending ? 'animate-spin' : ''}`}
                    />
                    {t('modelRefresh')}
                </Button>
            </div>
            <input type="hidden" value={formData.model} required />

            <div className="relative">
                <Input
                    ref={inputRef}
                    id={`${idPrefix}-model-custom`}
                    type="text"
                    value={inputValue}
                    onChange={(e) => setInputValue(e.target.value)}
                    onKeyDown={handleInputKeyDown}
                    placeholder={t('modelCustomPlaceholder')}
                    className="rounded-xl pr-10"
                />
                {inputValue.trim() &&
                    !customModels.includes(inputValue.trim()) &&
                    !autoModels.includes(inputValue.trim()) && (
                        <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => handleAddModel(inputValue)}
                            className="absolute top-1/2 right-1 h-7 w-7 -translate-y-1/2 rounded-lg p-0 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground"
                            title={t('modelAdd')}
                        >
                            <Plus className="size-4" />
                        </Button>
                    )}
            </div>

            <div className="space-y-2">
                <div className="flex items-center justify-between">
                    <label className="text-xs font-medium text-card-foreground">
                        {t('modelSelected')}{' '}
                        {autoModels.length + customModels.length > 0 &&
                            `(${autoModels.length + customModels.length})`}
                    </label>
                    {autoModels.length + customModels.length > 0 && (
                        <Button
                            type="button"
                            variant="ghost"
                            size="sm"
                            onClick={() => updateModels([], [])}
                            className="h-6 px-2 text-xs text-muted-foreground/50 hover:bg-transparent hover:text-muted-foreground"
                        >
                            {t('modelClearAll')}
                        </Button>
                    )}
                </div>
                <div className="max-h-40 min-h-12 overflow-y-auto rounded-xl border border-border bg-muted/30 p-2.5">
                    {autoModels.length + customModels.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {autoModels.map((model) => (
                                <Badge
                                    key={model}
                                    variant="secondary"
                                    className="bg-muted hover:bg-muted/80"
                                >
                                    {model}
                                    <button
                                        type="button"
                                        onClick={() =>
                                            updateModels(
                                                autoModels.filter((m) => m !== model),
                                                customModels,
                                            )
                                        }
                                        className="ml-1 rounded-sm opacity-70 hover:opacity-100 focus:ring-1 focus:ring-ring focus:outline-none"
                                    >
                                        <X className="h-3 w-3" />
                                    </button>
                                </Badge>
                            ))}
                            {customModels.map((model) => (
                                <Badge key={model} className="bg-primary hover:bg-primary/90">
                                    {model}
                                    <button
                                        type="button"
                                        onClick={() =>
                                            updateModels(
                                                autoModels,
                                                customModels.filter((m) => m !== model),
                                            )
                                        }
                                        className="ml-1 rounded-sm opacity-70 hover:opacity-100 focus:ring-1 focus:ring-ring focus:outline-none"
                                    >
                                        <X className="h-3 w-3" />
                                    </button>
                                </Badge>
                            ))}
                        </div>
                    ) : (
                        <div className="flex h-8 items-center justify-center text-xs text-muted-foreground">
                            {t('modelNoSelected')}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
