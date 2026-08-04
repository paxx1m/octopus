import {
    AutoGroupType,
    ChannelType,
    KeyMode,
    type Channel,
    type ChannelKey,
} from '@/api/endpoints/channel';

export interface ChannelKeyFormItem {
    id?: number;
    enabled: boolean;
    channel_key: string;
    status_code?: number;
    last_use_time_stamp?: number;
    total_cost?: number;
    remark?: string;
    weight?: number;
    rate_limit_cooldown_sec?: number | '';
}

export interface ChannelFormData {
    name: string;
    type: ChannelType;
    base_urls: Channel['base_urls'];
    custom_header: Channel['custom_header'];
    channel_proxy: string;
    param_override: string;
    keys: ChannelKeyFormItem[];
    model: string;
    custom_model: string;
    enabled: boolean;
    proxy: boolean;
    auto_sync: boolean;
    auto_group: AutoGroupType;
    match_regex: string;
    key_mode: KeyMode;
    rate_limit_cooldown_sec: number | '';
    allow_empty_key: boolean;
}

/** Empty key row used by create/edit forms. */
export function defaultKeyItem(partial?: Partial<ChannelKeyFormItem>): ChannelKeyFormItem {
    return {
        enabled: true,
        channel_key: '',
        remark: '',
        weight: 1,
        rate_limit_cooldown_sec: '',
        ...partial,
    };
}

/** Blank form for creating a channel. */
export function emptyChannelForm(): ChannelFormData {
    return {
        name: '',
        type: ChannelType.OpenAIChat,
        base_urls: [{ url: '', delay: 0 }],
        custom_header: [],
        channel_proxy: '',
        param_override: '',
        keys: [defaultKeyItem()],
        model: '',
        custom_model: '',
        auto_sync: false,
        auto_group: AutoGroupType.None,
        enabled: true,
        proxy: false,
        match_regex: '',
        key_mode: KeyMode.LeastCost,
        rate_limit_cooldown_sec: '',
        allow_empty_key: false,
    };
}

/** Map API channel into form state for edit. */
export function channelToFormData(channel: Channel): ChannelFormData {
    return {
        name: channel.name,
        type: channel.type,
        enabled: channel.enabled,
        base_urls: channel.base_urls?.length ? channel.base_urls : [{ url: '', delay: 0 }],
        custom_header: channel.custom_header ?? [],
        channel_proxy: channel.channel_proxy ?? '',
        param_override: channel.param_override ?? '',
        keys: channel.keys.length > 0
            ? channel.keys.map(keyToFormItem)
            : [defaultKeyItem()],
        model: channel.model,
        custom_model: channel.custom_model,
        proxy: channel.proxy,
        auto_sync: channel.auto_sync,
        auto_group: channel.auto_group,
        match_regex: channel.match_regex ?? '',
        key_mode: channel.key_mode || KeyMode.LeastCost,
        rate_limit_cooldown_sec: channel.rate_limit_cooldown_sec ?? '',
        allow_empty_key: channel.allow_empty_key ?? false,
    };
}

export function keyToFormItem(k: ChannelKey): ChannelKeyFormItem {
    return {
        id: k.id,
        enabled: k.enabled,
        channel_key: k.channel_key,
        status_code: k.status_code,
        last_use_time_stamp: k.last_use_time_stamp,
        total_cost: k.total_cost,
        remark: k.remark,
        weight: k.weight ?? 1,
        rate_limit_cooldown_sec: k.rate_limit_cooldown_sec ?? '',
    };
}

/** Form empty string / undefined → null (inherit); otherwise number. */
export function normalizeCooldown(value: number | '' | undefined | null): number | null {
    if (value === '' || value === undefined || value === null) return null;
    return Number(value);
}

export function normalizeWeight(value: number | undefined): number {
    return value && value > 0 ? value : 1;
}

/** Payload fields for create / keys_to_add. */
export function keyFormToAddPayload(k: ChannelKeyFormItem) {
    return {
        enabled: k.enabled,
        channel_key: k.channel_key,
        remark: k.remark ?? '',
        weight: normalizeWeight(k.weight),
        rate_limit_cooldown_sec: normalizeCooldown(k.rate_limit_cooldown_sec),
    };
}

export function normalizeBaseUrls(urls: Channel['base_urls'] | undefined) {
    return (urls ?? [])
        .filter((u) => u.url.trim())
        .map((u) => ({ url: u.url.trim(), delay: Number(u.delay || 0) }));
}

export function normalizeHeaders(headers: Channel['custom_header'] | undefined) {
    return (headers ?? [])
        .map((h) => ({ header_key: h.header_key.trim(), header_value: h.header_value }))
        .filter((h) => h.header_key && h.header_value !== '');
}

/**
 * Diff cooldown field for patch APIs.
 * clear=true means set NULL (inherit); value set means explicit seconds.
 */
export function cooldownPatch(
    next: number | '' | undefined | null,
    current: number | null | undefined,
): { clear?: true; value?: number } | null {
    const nextN = normalizeCooldown(next);
    const curN = current ?? null;
    if (nextN === curN) return null;
    if (nextN === null) return { clear: true };
    return { value: nextN };
}
