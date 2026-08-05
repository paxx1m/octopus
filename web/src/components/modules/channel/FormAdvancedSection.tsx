import { useTranslations } from 'use-intl';
import { Plus, X } from 'lucide-react';
import { AutoGroupType, type Channel } from '@/api/endpoints/channel';
import {
    Accordion,
    AccordionContent,
    AccordionItem,
    AccordionTrigger,
} from '@/components/ui/accordion';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select';
import type { ChannelFormData } from './form-utils';

type Props = {
    formData: ChannelFormData;
    onFormDataChange: (data: ChannelFormData) => void;
    idPrefix: string;
};

export function FormAdvancedSection({ formData, onFormDataChange, idPrefix }: Props) {
    const t = useTranslations('channel.form');

    const handleAddHeader = () => {
        onFormDataChange({
            ...formData,
            custom_header: [
                ...(formData.custom_header ?? []),
                { header_key: '', header_value: '' },
            ],
        });
    };

    const handleUpdateHeader = (
        idx: number,
        patch: Partial<Channel['custom_header'][number]>,
    ) => {
        const next = (formData.custom_header ?? []).map((h, i) =>
            i === idx ? { ...h, ...patch } : h,
        );
        onFormDataChange({ ...formData, custom_header: next });
    };

    const handleRemoveHeader = (idx: number) => {
        const curr = formData.custom_header ?? [];
        if (curr.length <= 1) return;
        onFormDataChange({
            ...formData,
            custom_header: curr.filter((_, i) => i !== idx),
        });
    };

    return (
        <Accordion type="single" collapsible className="w-full rounded-xl border bg-card">
            <AccordionItem value="advanced" className="border-none">
                <AccordionTrigger className="rounded-xl px-4 py-3 text-sm font-medium text-card-foreground transition-colors hover:bg-muted/30 hover:no-underline">
                    {t('advanced')}
                </AccordionTrigger>
                <AccordionContent className="space-y-4 border-t px-4 pt-4 pb-4">
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                        <div className="space-y-2">
                            <label
                                htmlFor={`${idPrefix}-auto-group`}
                                className="text-sm font-medium text-card-foreground"
                            >
                                {t('autoGroup')}
                            </label>
                            <Select
                                value={String(formData.auto_group)}
                                onValueChange={(value) =>
                                    onFormDataChange({
                                        ...formData,
                                        auto_group: Number(value) as AutoGroupType,
                                    })
                                }
                            >
                                <SelectTrigger
                                    id={`${idPrefix}-auto-group`}
                                    className="w-full rounded-xl border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                >
                                    <SelectValue />
                                </SelectTrigger>
                                <SelectContent className="rounded-xl">
                                    <SelectItem
                                        className="rounded-xl"
                                        value={String(AutoGroupType.None)}
                                    >
                                        {t('autoGroupNone')}
                                    </SelectItem>
                                    <SelectItem
                                        className="rounded-xl"
                                        value={String(AutoGroupType.Fuzzy)}
                                    >
                                        {t('autoGroupFuzzy')}
                                    </SelectItem>
                                    <SelectItem
                                        className="rounded-xl"
                                        value={String(AutoGroupType.Exact)}
                                    >
                                        {t('autoGroupExact')}
                                    </SelectItem>
                                    <SelectItem
                                        className="rounded-xl"
                                        value={String(AutoGroupType.Regex)}
                                    >
                                        {t('autoGroupRegex')}
                                    </SelectItem>
                                </SelectContent>
                            </Select>
                        </div>

                        <div className="space-y-2">
                            <label
                                htmlFor={`${idPrefix}-channel-proxy`}
                                className="text-sm font-medium text-card-foreground"
                            >
                                {t('channelProxy')}
                            </label>
                            <Input
                                id={`${idPrefix}-channel-proxy`}
                                type="text"
                                value={formData.channel_proxy}
                                onChange={(e) =>
                                    onFormDataChange({
                                        ...formData,
                                        channel_proxy: e.target.value,
                                    })
                                }
                                placeholder={t('channelProxyPlaceholder')}
                                className="rounded-xl"
                            />
                        </div>
                    </div>

                    <div className="space-y-2">
                        <div className="flex items-center justify-between">
                            <label className="text-sm font-medium text-card-foreground">
                                {t('customHeader')}{' '}
                                {formData.custom_header.length > 0
                                    ? `(${formData.custom_header.length})`
                                    : ''}
                            </label>
                            <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={handleAddHeader}
                                className="h-6 px-2 text-xs text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground"
                            >
                                <Plus className="mr-1 h-3 w-3" />
                                {t('customHeaderAdd')}
                            </Button>
                        </div>
                        <div className="space-y-2">
                            {(formData.custom_header ?? []).map((h, idx) => (
                                <div key={`hdr-${idx}`} className="flex items-center gap-2">
                                    <Input
                                        type="text"
                                        value={h.header_key}
                                        onChange={(e) =>
                                            handleUpdateHeader(idx, {
                                                header_key: e.target.value,
                                            })
                                        }
                                        placeholder={t('customHeaderKey')}
                                        className="flex-1 rounded-xl"
                                    />
                                    <Input
                                        type="text"
                                        value={h.header_value}
                                        onChange={(e) =>
                                            handleUpdateHeader(idx, {
                                                header_value: e.target.value,
                                            })
                                        }
                                        placeholder={t('customHeaderValue')}
                                        className="flex-1 rounded-xl"
                                    />
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        size="sm"
                                        onClick={() => handleRemoveHeader(idx)}
                                        disabled={(formData.custom_header ?? []).length <= 1}
                                        className="h-8 w-8 rounded-xl p-0 text-muted-foreground hover:bg-transparent hover:text-destructive disabled:opacity-40"
                                        title="Remove"
                                    >
                                        <X className="h-4 w-4" />
                                    </Button>
                                </div>
                            ))}
                        </div>
                    </div>

                    <div className="space-y-2">
                        <label
                            htmlFor={`${idPrefix}-match-regex`}
                            className="text-sm font-medium text-card-foreground"
                        >
                            {t('matchRegex')}
                        </label>
                        <Input
                            id={`${idPrefix}-match-regex`}
                            type="text"
                            value={formData.match_regex}
                            onChange={(e) =>
                                onFormDataChange({
                                    ...formData,
                                    match_regex: e.target.value,
                                })
                            }
                            placeholder={t('matchRegexPlaceholder')}
                            className="rounded-xl"
                        />
                    </div>

                    <div className="space-y-2">
                        <label
                            htmlFor={`${idPrefix}-param-override`}
                            className="text-sm font-medium text-card-foreground"
                        >
                            {t('paramOverride')}
                        </label>
                        <textarea
                            id={`${idPrefix}-param-override`}
                            value={formData.param_override}
                            onChange={(e) =>
                                onFormDataChange({
                                    ...formData,
                                    param_override: e.target.value,
                                })
                            }
                            placeholder={t('paramOverridePlaceholder')}
                            className="min-h-28 w-full rounded-xl border border-border bg-background px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        />
                    </div>
                </AccordionContent>
            </AccordionItem>
        </Accordion>
    );
}
