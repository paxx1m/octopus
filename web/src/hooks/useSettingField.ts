import { useEffect, useRef, useState } from 'react';
import { useTranslations } from 'use-intl';
import { useSettingList, useSetSetting } from '@/api/endpoints/setting';
import { toast } from '@/components/common/Toast';

/**
 * 单个 setting 字段的受控状态 + 自动保存。
 *
 * 封装了 useState + useRef + useEffect(settings 同步) + handleSave(onBlur 保存 + toast) 模板，
 * 消除 System/CircuitBreaker/Log/LLMPrice/LLMSync 中逐字段重复的状态管理代码。
 *
 * @example
 * const proxyUrl = useSettingField(SettingKey.ProxyURL);
 * <Input value={proxyUrl.value} onChange={e => proxyUrl.setValue(e.target.value)} onBlur={proxyUrl.save} />
 */
export function useSettingField(key: string) {
    const t = useTranslations('setting');
    const { data: settings } = useSettingList();
    const setSetting = useSetSetting();

    const [value, setValue] = useState('');
    const initialValue = useRef('');

    useEffect(() => {
        if (settings) {
            const item = settings.find(s => s.key === key);
            if (item) {
                queueMicrotask(() => setValue(item.value));
                initialValue.current = item.value;
            }
        }
    }, [settings, key]);

    const save = () => {
        if (value === initialValue.current) return;
        setSetting.mutate(
            { key, value },
            {
                onSuccess: () => {
                    toast.success(t('saved'));
                    initialValue.current = value;
                },
            },
        );
    };

    /** 直接保存指定值（用于非 onBlur 交互，如 CORS 增删后立即保存）。 */
    const saveValue = (v: string) => {
        if (v === initialValue.current) return;
        setValue(v);
        setSetting.mutate(
            { key, value: v },
            {
                onSuccess: () => {
                    toast.success(t('saved'));
                    initialValue.current = v;
                },
            },
        );
    };

    return { value, setValue, save, saveValue, isPending: setSetting.isPending };
}
