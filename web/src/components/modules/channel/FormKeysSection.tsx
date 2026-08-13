import { useMemo, useState } from 'react';
import { useTranslations } from 'use-intl';
import { ChevronDown, ListPlus, Plus, X } from 'lucide-react';
import { KeyMode } from '@/api/endpoints/channel';
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
import { toast } from '@/components/common/Toast';
import { cn } from '@/lib/utils';
import {
    defaultKeyItem,
    maskChannelKey,
    normalizeWeight,
    parseKeyLine,
    parseKeyLines,
    type ChannelFormData,
    type ChannelKeyFormItem,
} from './form-utils';

type Props = {
    formData: ChannelFormData;
    onFormDataChange: (data: ChannelFormData) => void;
    idPrefix: string;
};

function keyRowId(k: ChannelKeyFormItem, idx: number) {
    return typeof k.id === 'number' ? `id-${k.id}` : `new-${idx}`;
}

export function FormKeysSection({ formData, onFormDataChange, idPrefix }: Props) {
    const t = useTranslations('channel.form');
    const tc = useTranslations('common');
    const [keyBulkMode, setKeyBulkMode] = useState(false);
    const [bulkText, setBulkText] = useState('');
    const [expandedKeyIds, setExpandedKeyIds] = useState<Set<string>>(() => new Set());

    const isWeighted = formData.key_mode === KeyMode.Weighted;
    const existingKeyCount = useMemo(
        () => (formData.keys ?? []).filter((k) => typeof k.id === 'number').length,
        [formData.keys],
    );
    const useSingleKeyUi =
        !keyBulkMode && existingKeyCount === 0 && (formData.keys?.length ?? 0) <= 1;
    const bulkPreviewCount = useMemo(() => parseKeyLines(bulkText).length, [bulkText]);
    const filledKeyCount = useMemo(
        () => (formData.keys ?? []).filter((k) => k.channel_key.trim()).length,
        [formData.keys],
    );

    const toggleKeyExpanded = (rowId: string) => {
        setExpandedKeyIds((prev) => {
            const next = new Set(prev);
            if (next.has(rowId)) next.delete(rowId);
            else next.add(rowId);
            return next;
        });
    };

    const handleAddKey = () => {
        onFormDataChange({
            ...formData,
            keys: [...formData.keys, defaultKeyItem()],
        });
    };

    const handleUpdateKey = (idx: number, patch: Partial<ChannelKeyFormItem>) => {
        const next = formData.keys.map((k, i) => (i === idx ? { ...k, ...patch } : k));
        onFormDataChange({ ...formData, keys: next });
    };

    const handleRemoveKey = (idx: number) => {
        const curr = formData.keys ?? [];
        if (curr.length <= 1) {
            onFormDataChange({ ...formData, keys: [defaultKeyItem()] });
            return;
        }
        const next = curr.filter((_, i) => i !== idx);
        onFormDataChange({ ...formData, keys: next.length ? next : [defaultKeyItem()] });
    };

    const applyBulkKeys = (mode: 'replace' | 'append') => {
        const parsed = parseKeyLines(bulkText);
        if (parsed.length === 0) {
            toast.error(t('bulkKeysEmpty'));
            return;
        }
        const existing = (formData.keys ?? []).filter((k) => typeof k.id === 'number');
        const existingSet = new Set(existing.map((k) => k.channel_key));
        const listKeys =
            mode === 'append'
                ? (formData.keys ?? []).filter((k) => k.channel_key.trim())
                : existing;
        const seen = new Set(listKeys.map((k) => k.channel_key));
        const toAdd = parsed.filter((p) => {
            if (seen.has(p.channel_key) || existingSet.has(p.channel_key)) return false;
            seen.add(p.channel_key);
            return true;
        });
        if (toAdd.length === 0) {
            toast.error(t('bulkKeysNoNew'));
            return;
        }
        const next =
            mode === 'append'
                ? [...listKeys, ...toAdd]
                : existing.length
                  ? [...existing, ...toAdd]
                  : toAdd;
        onFormDataChange({ ...formData, keys: next });
        setBulkText('');
        setKeyBulkMode(false);
        toast.success(t('bulkKeysApplied', { count: toAdd.length }));
    };

    const handleSingleKeyBlur = () => {
        if (!useSingleKeyUi) return;
        const raw = formData.keys?.[0]?.channel_key ?? '';
        if (!raw.includes('|')) return;
        const parsed = parseKeyLine(raw);
        if (!parsed) return;
        onFormDataChange({
            ...formData,
            keys: [{ ...formData.keys[0], ...parsed, id: formData.keys[0]?.id }],
        });
    };

    if (formData.allow_empty_key) return null;

    return (
        <>
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                    <label
                        htmlFor={`${idPrefix}-key-mode`}
                        className="text-sm font-medium text-card-foreground"
                    >
                        {t('keyMode')}
                    </label>
                    <Select
                        value={String(formData.key_mode)}
                        onValueChange={(value) =>
                            onFormDataChange({
                                ...formData,
                                key_mode: Number(value) as KeyMode,
                            })
                        }
                    >
                        <SelectTrigger
                            id={`${idPrefix}-key-mode`}
                            className="w-full rounded-xl border border-border px-4 py-2 text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        >
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl">
                            <SelectItem className="rounded-xl" value={String(KeyMode.LeastCost)}>
                                {t('keyModeLeastCost')}
                            </SelectItem>
                            <SelectItem className="rounded-xl" value={String(KeyMode.RoundRobin)}>
                                {t('keyModeRoundRobin')}
                            </SelectItem>
                            <SelectItem className="rounded-xl" value={String(KeyMode.Random)}>
                                {t('keyModeRandom')}
                            </SelectItem>
                            <SelectItem className="rounded-xl" value={String(KeyMode.Failover)}>
                                {t('keyModeFailover')}
                            </SelectItem>
                            <SelectItem className="rounded-xl" value={String(KeyMode.Weighted)}>
                                {t('keyModeWeighted')}
                            </SelectItem>
                        </SelectContent>
                    </Select>
                </div>
                <div className="space-y-2">
                    <label
                        htmlFor={`${idPrefix}-rate-limit`}
                        className="text-sm font-medium text-card-foreground"
                    >
                        {t('rateLimitCooldown')}
                    </label>
                    <Input
                        id={`${idPrefix}-rate-limit`}
                        type="number"
                        min={0}
                        value={
                            formData.rate_limit_cooldown_sec === ''
                                ? ''
                                : formData.rate_limit_cooldown_sec
                        }
                        onChange={(e) => {
                            const v = e.target.value;
                            onFormDataChange({
                                ...formData,
                                rate_limit_cooldown_sec: v === '' ? '' : Number(v),
                            });
                        }}
                        placeholder={t('rateLimitCooldownPlaceholder')}
                        className="rounded-xl"
                    />
                </div>
            </div>

            <div className="space-y-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                    <label className="text-sm font-medium text-card-foreground">
                        {t('apiKey')}
                        {filledKeyCount > 0 ? ` (${filledKeyCount})` : ''}
                    </label>
                    <div className="flex items-center gap-1">
                        {!keyBulkMode && (
                            <>
                                <Button
                                    type="button"
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => setKeyBulkMode(true)}
                                    className="h-6 px-2 text-xs text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground"
                                >
                                    <ListPlus className="mr-1 h-3 w-3" />
                                    {t('bulkImport')}
                                </Button>
                                {!useSingleKeyUi && (
                                    <Button
                                        type="button"
                                        variant="ghost"
                                        size="sm"
                                        onClick={handleAddKey}
                                        className="h-6 px-2 text-xs text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground"
                                    >
                                        <Plus className="mr-1 h-3 w-3" />
                                        {t('add')}
                                    </Button>
                                )}
                            </>
                        )}
                        {keyBulkMode && (
                            <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={() => {
                                    setKeyBulkMode(false);
                                    setBulkText('');
                                }}
                                className="h-6 px-2 text-xs text-muted-foreground/70 hover:bg-transparent hover:text-muted-foreground"
                            >
                                {t('bulkCancel')}
                            </Button>
                        )}
                    </div>
                </div>

                {keyBulkMode ? (
                    <div className="space-y-2 rounded-xl border border-border/60 bg-muted/10 p-3">
                        <textarea
                            value={bulkText}
                            onChange={(e) => setBulkText(e.target.value)}
                            rows={6}
                            placeholder={t('bulkKeysPlaceholder')}
                            className="min-h-28 w-full rounded-xl border border-border bg-background px-3 py-2 font-mono text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        />
                        <p className="text-[11px] leading-relaxed text-muted-foreground">
                            {t('bulkKeysHint')}
                        </p>
                        <div className="flex flex-wrap items-center justify-between gap-2">
                            <span className="text-xs text-muted-foreground">
                                {bulkPreviewCount > 0
                                    ? t('bulkKeysPreview', { count: bulkPreviewCount })
                                    : t('bulkKeysPreviewEmpty')}
                            </span>
                            <Button
                                type="button"
                                size="sm"
                                className="rounded-xl"
                                disabled={bulkPreviewCount === 0}
                                onClick={() =>
                                    applyBulkKeys(existingKeyCount > 0 ? 'append' : 'replace')
                                }
                            >
                                {existingKeyCount > 0 ? t('bulkKeysAppend') : t('bulkKeysApply')}
                            </Button>
                        </div>
                    </div>
                ) : useSingleKeyUi ? (
                    <div className="space-y-2">
                        <Input
                            type="text"
                            value={formData.keys[0]?.channel_key ?? ''}
                            onChange={(e) => handleUpdateKey(0, { channel_key: e.target.value })}
                            onBlur={handleSingleKeyBlur}
                            placeholder={t('apiKey')}
                            required
                            className="rounded-xl"
                        />
                        {isWeighted && (
                            <div className="flex items-center gap-2">
                                <label className="shrink-0 text-xs text-muted-foreground">
                                    {t('weight')}
                                </label>
                                <Input
                                    type="number"
                                    min={1}
                                    value={normalizeWeight(formData.keys[0]?.weight)}
                                    onChange={(e) =>
                                        handleUpdateKey(0, {
                                            weight: normalizeWeight(Number(e.target.value)),
                                        })
                                    }
                                    className="w-24 rounded-xl"
                                />
                            </div>
                        )}
                        <p className="text-[11px] text-muted-foreground">{t('singleKeyHint')}</p>
                    </div>
                ) : (
                    <div className="space-y-2">
                        {(formData.keys ?? []).map((k, idx) => {
                            const rowId = keyRowId(k, idx);
                            const expanded = expandedKeyIds.has(rowId);
                            return (
                                <div
                                    key={rowId}
                                    className="overflow-hidden rounded-xl border border-border/50 bg-muted/10"
                                >
                                    <div className="flex flex-wrap items-center gap-2 p-2">
                                        <Input
                                            type="text"
                                            value={k.channel_key}
                                            onChange={(e) =>
                                                handleUpdateKey(idx, {
                                                    channel_key: e.target.value,
                                                })
                                            }
                                            placeholder={t('apiKey')}
                                            required={idx === 0}
                                            className="min-w-[8rem] flex-1 rounded-xl font-mono text-sm"
                                            title={k.channel_key || undefined}
                                        />
                                        {isWeighted && (
                                            <Input
                                                type="number"
                                                min={1}
                                                value={normalizeWeight(k.weight)}
                                                onChange={(e) =>
                                                    handleUpdateKey(idx, {
                                                        weight: normalizeWeight(
                                                            Number(e.target.value),
                                                        ),
                                                    })
                                                }
                                                placeholder={t('weight')}
                                                className="w-16 rounded-xl"
                                                title={t('weight')}
                                            />
                                        )}
                                        <Switch
                                            checked={k.enabled}
                                            onCheckedChange={(checked) =>
                                                handleUpdateKey(idx, { enabled: checked })
                                            }
                                        />
                                        <Button
                                            type="button"
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => toggleKeyExpanded(rowId)}
                                            className="h-8 w-8 rounded-xl p-0 text-muted-foreground hover:bg-transparent"
                                            title={t('keyAdvanced')}
                                        >
                                            <ChevronDown
                                                className={cn(
                                                    'h-4 w-4 transition-transform',
                                                    expanded && 'rotate-180',
                                                )}
                                            />
                                        </Button>
                                        <Button
                                            type="button"
                                            variant="ghost"
                                            size="sm"
                                            onClick={() => handleRemoveKey(idx)}
                                            className="h-8 w-8 rounded-xl p-0 text-muted-foreground hover:bg-transparent hover:text-destructive"
                                            title={tc("remove")}
                                        >
                                            <X className="h-4 w-4" />
                                        </Button>
                                    </div>
                                    {expanded && (
                                        <div className="grid grid-cols-1 gap-2 border-t border-border/40 bg-background/40 p-2 sm:grid-cols-3">
                                            <Input
                                                type="text"
                                                value={k.remark ?? ''}
                                                onChange={(e) =>
                                                    handleUpdateKey(idx, {
                                                        remark: e.target.value,
                                                    })
                                                }
                                                placeholder={t('remark')}
                                                className="rounded-xl"
                                            />
                                            {!isWeighted && (
                                                <Input
                                                    type="number"
                                                    min={1}
                                                    value={normalizeWeight(k.weight)}
                                                    onChange={(e) =>
                                                        handleUpdateKey(idx, {
                                                            weight: normalizeWeight(
                                                                Number(e.target.value),
                                                            ),
                                                        })
                                                    }
                                                    placeholder={t('weight')}
                                                    className="rounded-xl"
                                                    title={t('weight')}
                                                />
                                            )}
                                            <Input
                                                type="number"
                                                min={0}
                                                value={
                                                    k.rate_limit_cooldown_sec === '' ||
                                                    k.rate_limit_cooldown_sec === undefined
                                                        ? ''
                                                        : k.rate_limit_cooldown_sec
                                                }
                                                onChange={(e) => {
                                                    const v = e.target.value;
                                                    handleUpdateKey(idx, {
                                                        rate_limit_cooldown_sec:
                                                            v === '' ? '' : Number(v),
                                                    });
                                                }}
                                                placeholder={t('keyCooldown')}
                                                className="rounded-xl"
                                                title={t('keyCooldown')}
                                            />
                                            {k.channel_key.trim() && (
                                                <p className="text-[11px] text-muted-foreground sm:col-span-3">
                                                    {maskChannelKey(k.channel_key)}
                                                    {k.remark ? ` · ${k.remark}` : ''}
                                                </p>
                                            )}
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                    </div>
                )}
            </div>
        </>
    );
}
