import type { LLMChannel } from '@/api/endpoints/model';
import {
    GroupMode,
    type Group,
    type GroupUpdateRequest,
} from '@/api/endpoints/group';
import type { SelectedMember } from './ItemList';

export const MODE_LABELS: Record<GroupMode, string> = {
    [GroupMode.RoundRobin]: 'roundRobin',
    [GroupMode.Random]: 'random',
    [GroupMode.Failover]: 'failover',
    [GroupMode.Weighted]: 'weighted',
} as const;

export function normalizeKey(value: string) {
    return value.trim().toLowerCase();
}

export function modelChannelKey(channelId: number, modelName: string) {
    return `${channelId}-${modelName}`;
}

export function memberKey(member: Pick<LLMChannel, 'channel_id' | 'name'>) {
    return modelChannelKey(member.channel_id, member.name);
}

export function matchesGroupName(modelName: string, groupKey: string) {
    if (!groupKey) return false;
    return modelName.toLowerCase().includes(groupKey);
}

export function buildChannelNameByModelKey(modelChannels: LLMChannel[]) {
    const map = new Map<string, string>();
    modelChannels.forEach((mc) => {
        map.set(modelChannelKey(mc.channel_id, mc.name), mc.channel_name);
    });
    return map;
}

/** Build a patch payload for group member reorder / weight changes. */
export function buildItemsUpdatePayload(
    groupId: number,
    members: SelectedMember[],
    priorityByItemId: Map<number, number>,
): GroupUpdateRequest | null {
    const items_to_update = members
        .map((m, i) => ({ member: m, newPriority: i + 1 }))
        .filter(({ member, newPriority }) => {
            if (!member.item_id) return false;
            const origPriority = priorityByItemId.get(member.item_id);
            return origPriority !== undefined && origPriority !== newPriority;
        })
        .map(({ member, newPriority }) => ({
            id: member.item_id!,
            priority: newPriority,
            weight: member.weight ?? 1,
        }));

    if (items_to_update.length === 0) return null;
    return { id: groupId, items_to_update };
}

/** Build full group update payload from editor values vs current group. */
export function buildGroupUpdatePayload(
    group: Group,
    values: {
        name: string;
        match_regex?: string;
        mode: GroupMode;
        first_token_time_out?: number;
        session_keep_time?: number;
        members: SelectedMember[];
    },
): GroupUpdateRequest | null {
    if (!group.id) return null;

    const originalItems = [...(group.items || [])].sort((a, b) => a.priority - b.priority);
    const originalById = new Map<number, { priority: number; weight: number }>();
    const originalIds = new Set<number>();
    originalItems.forEach((it) => {
        if (typeof it.id === 'number') {
            originalIds.add(it.id);
            originalById.set(it.id, { priority: it.priority, weight: it.weight });
        }
    });

    const newIds = new Set<number>();
    values.members.forEach((m) => {
        if (typeof m.item_id === 'number') newIds.add(m.item_id);
    });

    const items_to_delete = Array.from(originalIds).filter((id) => !newIds.has(id));

    const items_to_add = values.members
        .map((m, idx) => ({ m, priority: idx + 1 }))
        .filter(({ m }) => typeof m.item_id !== 'number')
        .map(({ m, priority }) => ({
            channel_id: m.channel_id,
            model_name: m.name,
            priority,
            weight: m.weight ?? 1,
        }));

    const items_to_update = values.members
        .map((m, idx) => ({ m, priority: idx + 1 }))
        .filter(({ m }) => typeof m.item_id === 'number')
        .map(({ m, priority }) => {
            const id = m.item_id!;
            const orig = originalById.get(id);
            const weight = m.weight ?? 1;
            if (!orig) return null;
            if (orig.priority === priority && orig.weight === weight) return null;
            return { id, priority, weight };
        })
        .filter((x): x is { id: number; priority: number; weight: number } => x !== null);

    const payload: GroupUpdateRequest = { id: group.id };
    const nextName = values.name.trim();
    const nextRegex = (values.match_regex ?? '').trim();
    const nextFirstTokenTimeOut = values.first_token_time_out ?? 0;
    const nextSessionKeepTime = values.session_keep_time ?? 0;

    if (nextName && nextName !== group.name) payload.name = nextName;
    if (values.mode !== group.mode) payload.mode = values.mode;
    if (nextRegex !== (group.match_regex ?? '')) payload.match_regex = nextRegex;
    if (nextFirstTokenTimeOut !== (group.first_token_time_out ?? 0)) {
        payload.first_token_time_out = nextFirstTokenTimeOut;
    }
    if (nextSessionKeepTime !== (group.session_keep_time ?? 0)) {
        payload.session_keep_time = nextSessionKeepTime;
    }
    if (items_to_add.length) payload.items_to_add = items_to_add;
    if (items_to_update.length) payload.items_to_update = items_to_update;
    if (items_to_delete.length) payload.items_to_delete = items_to_delete;

    if (Object.keys(payload).length === 1) return null;
    return payload;
}


