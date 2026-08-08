import type { APIKey } from '@/api/endpoints/apikey';

export function toExpireAt(date: Date, time: string): number {
    const t = /^\d{2}:\d{2}$/.test(time) ? time : '00:00';
    const [hh, mm] = t.split(':').map(Number);
    const d = new Date(Date.UTC(date.getFullYear(), date.getMonth(), date.getDate(), hh, mm, 0));
    return Math.floor(d.getTime() / 1000);
}

export function parseExpireDate(expireAt?: number): Date | undefined {
    if (!expireAt) return undefined;
    const d = new Date(expireAt * 1000);
    return isNaN(d.getTime()) ? undefined : d;
}

export function normalizeHHmm(input: string): string {
    const cleaned = input.replace(/[^\d:]/g, '');
    const parts = cleaned.includes(':') ? cleaned.split(':') : [cleaned.slice(0, 2), cleaned.slice(2, 4)];
    const hh = Math.min(23, Math.max(0, parseInt(parts[0] || '0', 10)));
    const mm = Math.min(59, Math.max(0, parseInt(parts[1] || '0', 10)));
    return `${hh.toString().padStart(2, '0')}:${mm.toString().padStart(2, '0')}`;
}

export function normalizeMoneyInput(input: string): string {
    const cleaned = input.replace(/[^\d.]/g, '');
    const [intPart, ...rest] = cleaned.split('.');
    return rest.length > 0 ? `${intPart}.${rest.join('').slice(0, 6)}` : intPart;
}

export function toggleModel(current: string | undefined, model: string): string | undefined {
    const models = current ? current.split(',').filter(Boolean) : [];
    const next = models.includes(model)
        ? models.filter((m) => m !== model)
        : [...models, model];
    return next.length ? next.join(',') : undefined;
}

export function hasModel(supported: string | undefined, model: string): boolean {
    return supported ? supported.split(',').includes(model) : false;
}

export type APIKeyFormData = Omit<APIKey, 'id' | 'api_key'>;
